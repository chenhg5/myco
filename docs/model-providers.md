# Model providers

**English** · [中文](model-providers.zh-CN.md) · [README](../README.md) · [Install guide](../INSTALL.md)

A model provider is your own API key for a model vendor, kept in the workspace. Add it once, then pick it whenever you start a Claude Code or Codex agent on any of your computers, instead of the login that computer happens to have.

Pick a built-in vendor — MiniMax, DeepSeek, Kimi, OpenCode Go, Zhipu, Qwen, Doubao or OpenRouter (Zhipu, Qwen and Doubao are labelled 智谱, 通义 and 豆包 in the App) — or **Custom**: any Anthropic- or OpenAI-compatible address. A provider can carry one address for Claude Code and one for Codex; at least one is needed.

Providers belong to you, not to the workspace. Each member adds their own, and no other member can see or use yours.

## Add one in the App

1. Open **Settings › Model providers**.
2. If there are none yet, choose **Add a model provider**; after that, **Add**.
3. Pick the vendor. For a built-in vendor its two addresses are filled in; for **Custom** ("An Anthropic- or OpenAI-compatible address") you enter them yourself.
4. Fill **Name** and **Key** with the key the vendor gave you. **Addresses and small model** unfolds **Anthropic address (for Claude Code)**, **OpenAI address (for Codex)** and **Small model (optional)**. Change an address only if the vendor gave you a different one. The small model is for Claude Code's background work; blank keeps the vendor's usual one.
5. **Test** reads the vendor's models with the key and shows, for example, "3 models: …". **Save** keeps the provider.

You can also add one without leaving the add-agent form: see **A new agent** below.

The section says what happens to the key. Who keeps it depends on whether the workspace has end-to-end encryption on, which is your choice; providers work either way (see **Who holds the key**):

> With encryption on: Keys are encrypted on this computer before they are kept in the workspace: only your own devices open them, the workspace cannot. When you add an agent, the key is written only to that computer (its env file, mode 600).

> With encryption off: Keys are kept on the hub, where its administrator could see them; for a shared team, turn on end-to-end encryption. **Turn on encryption** goes to Settings › Workspaces.

## Use it for an agent

**A new agent.** In the add-agent form, choose **Which AI**; right under it, **Where its model comes from**. The first choice, "“computer”'s own sign-in or settings", leaves that computer's own configuration alone. Your providers with an address for the chosen CLI come next. **+ Use your own API key** adds one right there: choose the vendor (or **A custom address**), enter the key, **Test**, then **Save and use**; it is kept in Settings › Model providers and picked. With a provider picked, **Model** lists the models read from that vendor (**Its default model** unless you want a particular one), and one line says who keeps the key.

**An agent that already exists on another computer.** Open the agent and its **Provider** row. Choose the provider labelled **My model provider**, then choose **Model** (**Its default** unless you want a particular one). **Follow its computer** puts it back on that computer's own setting. This changes only that agent. For an agent on this computer, the **Provider** row edits this computer's own provider setting instead.

The models offered come from the last **Test** of that provider. If none was read yet, leave **Model** empty and the CLI's default is used; **Test** in **Settings › Model providers** reads the list.

## Change the key

**Edit** the provider, enter the new key in **Key**, and **Save**. "A new key replaces it; agents that use it are offered the new one."

A provider whose agents still have the old key shows **Push new key (N)**; an agent's **Provider** row also shows "“name” has a new key; it still has the old one · **Push**". After a push the App says "The new key went to N agents; they use it from their next message." Agents that could not be reached are named with the error.

## Who holds the key

**With end-to-end encryption on:**

- The key is sealed on your computer before it reaches the workspace. Only your own devices can open it; the hub cannot read it and never logs it. The workspace also keeps the name and addresses, so it can show them.
- To an agent, the key goes only to the computer that runs it: it is opened on your computer, sealed again for that computer, and written to the agent's env file (`~/.myco/env/<agent>.env`, mode 600). That file is the copy the agent uses.
- A device added later cannot open the key by itself. Open the App once on another of your devices that can; it seals the key again for the new one. Until then the provider says **This device cannot open the key yet**. If no device that could open it is left, enter the key again with **Edit**.
**With end-to-end encryption off** (fine for a workspace of your own on your own hub; for a team, turn it on):

- The hub keeps the key, encrypted with the workspace's own key: a file `provider-secret.key` next to the workspace's database, mode 600. The hub's administrator could read it. The provider says **Kept by the hub**.
- The hub sends the key only to the runner of the agent you picked it for (or the computer starting that agent), over HTTPS; it never appears in the agent's settings, in messages or in logs. Each time it is handed out is recorded, without the key.
- If that key file is lost (a hub moved without it), the provider says **key lost**: **Edit** it and enter the key again, or restore the hub from a backup.

**Turning encryption on or off later.** On: the App, next time it lists your providers, encrypts their keys for your devices, and the hub drops its copy. Off: a provider says the workspace has encryption off now and offers **Hand to the hub**, which asks you first. Agents already set up keep what is on their computers either way.

- Treat backups as secrets. A hub's backup holds the workspace's key file with its database; and a computer that runs an agent has the key it uses in its env file, so a backup of that computer contains it.
- Your providers are yours alone: other members cannot see or use them.

## FAQ

### The test failed. What does the message mean?

The message names the vendor's host and the HTTP status only, for example `models from api.deepseek.com: HTTP 401`; it never contains the key. A 401 or 403 usually means the vendor refused the key; a 404 usually means the address is wrong; a 5xx, `unreachable`, `cut off` or a timeout is on the vendor's side or the network. Check the key and the address, then whether this computer can reach the vendor.

### I have a new computer or a new device. What do I do?

Open the App once on a device that can already open the key; it seals the key again for the new device, and the provider row stops saying **This device cannot open the key yet**. If no such device remains, enter the key again with **Edit** and **Save**.

### What happens to agents if I delete a provider?

The confirmation says: "N agents use it. They keep using what is written on their computers; the App asks you to pick another or clear it." Those agents keep running with the copy on their computer, and the agent's **Provider** row says: "Its model provider was deleted: what is on its computer still works. Pick another, or have it follow its computer again." New agents will not offer the deleted provider. Deleting it in the App does not reach into those computers; pick another provider for the agent, or set it back to following its computer.
