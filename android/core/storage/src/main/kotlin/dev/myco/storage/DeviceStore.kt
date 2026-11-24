package dev.myco.storage

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.AtomicFile
import android.util.Base64
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import dev.myco.model.*
import java.io.File
import java.security.KeyStore
import java.security.MessageDigest
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString

private val Context.settings by preferencesDataStore("device")

/** Credentials, drafts and cached conversations are encrypted with a device-only key. */
class DeviceStore(context: Context) : WorkspaceStore, ConversationStore {
    private val context = context.applicationContext
    private val lock = Mutex()
    private val crypto = DeviceCipher()
    private val accountsKey = stringPreferencesKey("workspaces")
    private val selectedKey = stringPreferencesKey("selected")
    private val preferencesKey = stringPreferencesKey("appearance_language")

    val preferences =
        context.settings.data.map { prefs ->
            prefs[preferencesKey]?.let { HubJson.decodeFromString<AppPreferences>(it).normalized() }
                ?: AppPreferences()
        }

    /**
     * The edit reads the latest value so quick changes to different fields do not overwrite one
     * another.
     */
    suspend fun updatePreferences(change: (AppPreferences) -> AppPreferences) {
        context.settings.edit { prefs ->
            val before =
                prefs[preferencesKey]?.let { HubJson.decodeFromString<AppPreferences>(it) }
                    ?: AppPreferences()
            prefs[preferencesKey] = HubJson.encodeToString(change(before.normalized()).normalized())
        }
    }

    suspend fun rename(id: String, name: String): Workspace = lock.withLock {
        val clean = name.trim()
        require(clean.isNotEmpty() && clean.length <= 80 && clean.none { it.isISOControl() }) {
            "Use a name of 1–80 characters"
        }
        val all = accounts()
        val before = all.firstOrNull { it.workspace.id == id } ?: error("Workspace not found")
        val changed = before.copy(workspace = before.workspace.copy(name = clean))
        context.settings.edit {
            it[accountsKey] =
                crypto.seal(
                    HubJson.encodeToString(
                        all.map { account ->
                            if (account.workspace.id == id) changed else account
                        }
                    )
                )
        }
        changed.workspace
    }

    private suspend fun accounts(): List<Credentials> {
        val text = context.settings.data.first()[accountsKey] ?: return emptyList()
        return HubJson.decodeFromString(crypto.open(text))
    }

    override suspend fun workspaces() = lock.withLock { accounts().map { it.workspace } }

    override suspend fun credentials(id: String) = lock.withLock {
        accounts().firstOrNull { it.workspace.id == id }
    }

    override suspend fun save(credentials: Credentials) = lock.withLock {
        val all = accounts().filterNot { it.workspace.id == credentials.workspace.id } + credentials
        val encrypted = crypto.seal(HubJson.encodeToString(all))
        context.settings.edit { it[accountsKey] = encrypted }
        Unit
    }

    override suspend fun remove(id: String) = lock.withLock {
        val all = accounts().filterNot { it.workspace.id == id }
        context.settings.edit { prefs ->
            prefs[accountsKey] = crypto.seal(HubJson.encodeToString(all))
            if (prefs[selectedKey] == id) prefs.remove(selectedKey)
        }
        withContext(Dispatchers.IO) { cacheFolder(id).deleteRecursively() }
        Unit
    }

    override suspend fun selected() = context.settings.data.first()[selectedKey]

    override suspend fun select(id: String?) {
        context.settings.edit { if (id == null) it.remove(selectedKey) else it[selectedKey] = id }
    }

    private fun hash(value: String) =
        MessageDigest.getInstance("SHA-256").digest(value.toByteArray()).joinToString("") {
            "%02x".format(it)
        }

    private fun cacheFolder(workspace: String) =
        File(context.noBackupFilesDir, "conversations/${hash(workspace)}")

    private fun cache(key: String, suffix: String) =
        File(cacheFolder(key.substringBefore(':')), "${hash(key)}.$suffix")

    private suspend fun read(key: String, suffix: String): String? =
        withContext(Dispatchers.IO) {
            val file = AtomicFile(cache(key, suffix))
            if (!file.baseFile.exists()) null else crypto.open(file.readFully().decodeToString())
        }

    private suspend fun write(key: String, suffix: String, text: String) =
        withContext(Dispatchers.IO) {
            val target = cache(key, suffix)
            target.parentFile!!.mkdirs()
            val file = AtomicFile(target)
            val stream = file.startWrite()
            try {
                stream.write(crypto.seal(text).toByteArray())
                file.finishWrite(stream)
            } catch (e: Exception) {
                file.failWrite(stream)
                throw e
            }
        }

    override suspend fun pending(key: String): List<PendingSend> = lock.withLock {
        read(key, "pending")?.let { HubJson.decodeFromString(it) } ?: emptyList()
    }

    override suspend fun savePending(key: String, pending: List<PendingSend>) = lock.withLock {
        write(key, "pending", HubJson.encodeToString(pending))
    }

    override suspend fun messages(key: String): List<Message> = lock.withLock {
        read(key, "messages")?.let { HubJson.decodeFromString(it) } ?: emptyList()
    }

    override suspend fun saveMessages(key: String, messages: List<Message>) = lock.withLock {
        write(key, "messages", HubJson.encodeToString(messages.takeLast(400)))
    }

    override suspend fun draft(key: String) = lock.withLock { read(key, "draft") ?: "" }

    override suspend fun saveDraft(key: String, text: String) = lock.withLock {
        write(key, "draft", text)
    }
}

private class DeviceCipher {
    private val key: SecretKey by lazy {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey("myco-device", null) as? SecretKey)
            ?: KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
                .apply {
                    init(
                        KeyGenParameterSpec.Builder(
                                "myco-device",
                                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
                            )
                            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                            .build()
                    )
                }
                .generateKey()
    }

    @Synchronized
    fun seal(text: String): String {
        val cipher =
            Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.ENCRYPT_MODE, key) }
        val data = cipher.doFinal(text.toByteArray())
        return Base64.encodeToString(cipher.iv + data, Base64.NO_WRAP)
    }

    @Synchronized
    fun open(text: String): String {
        val data = Base64.decode(text, Base64.NO_WRAP)
        require(data.size >= 28) { "Saved workspace data is damaged" }
        return Cipher.getInstance("AES/GCM/NoPadding").run {
            init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, data.copyOfRange(0, 12)))
            doFinal(data.copyOfRange(12, data.size)).decodeToString()
        }
    }
}
