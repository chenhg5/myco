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

The section says what happens to the key:

> Keys are encrypted on this computer before they are kept in the workspace: only your own devices open them, the workspace cannot. When you add an agent, the key is written only to that computer (its env file, mode 600).

If the workspace has not turned on end-to-end encryption, the section instead says **Turn on end-to-end encryption first** — "Keys are kept in the workspace encrypted for your own devices, so this workspace needs end-to-end encryption first." Choose **Turn it on**; or, as before, enter it on the agent's computer, in its **Provider** row.

## Use it for an agent

**A new agent.** In the add-agent form, choose **Which AI**, then **Which model provider**. The first choice, "“computer”'s own setting", leaves that computer's own configuration alone. Only providers with an address for the chosen CLI are listed. After you pick one, **Its default model** offers the models last read from that vendor; leave it for the default. The form notes: "The key is opened here and sealed again for “computer” alone, which writes it to the new agent's own settings."

**An agent that already exists on another computer.** Open the agent and its **Provider** row. Choose the provider labelled **My model provider**, then choose **Model** (**Its default** unless you want a particular one). **Follow its computer** puts it back on that computer's own setting. This changes only that agent. For an agent on this computer, the **Provider** row edits this computer's own provider setting instead.

The models offered come from the last **Test** of that provider. If none was read yet, leave **Model** empty and the CLI's default is used; **Test** in **Settings › Model providers** reads the list.

## Change the key

**Edit** the provider, enter the new key in **Key**, and **Save**. "A new key replaces it; agents that use it are offered the new one."

A provider whose agents still have the old key shows **Push new key (N)**; an agent's **Provider** row also shows "“name” has a new key; it still has the old one · **Push**". After a push the App says "The new key went to N agents; they use it from their next message." Agents that could not be reached are named with the error.

## Who holds the key

- The key is sealed on your computer before it reaches the workspace. Only your own devices can open it; the hub cannot read it and never logs it. The workspace also keeps the name and addresses, so it can show them.
- To an agent, the key goes only to the computer that runs it: it is opened on your computer, sealed again for that computer, and written to the agent's env file (`~/.myco/env/<agent>.env`, mode 600). That file is the copy the agent uses.
- A device added later cannot open the key by itself. Open the App once on another of your devices that can; it seals the key again for the new one. Until then the provider says **This device cannot open the key yet**. If no device that could open it is left, enter the key again with **Edit**.
- Treat backups as secrets. The workspace's copy of the key is sealed, but a computer that runs an agent has the key it uses in its env file, so a backup of that computer contains it.
- Your providers are yours alone: other members cannot see or use them.

## FAQ

### The test failed. What does the message mean?

The message names the vendor's host and the HTTP status only, for example `models from api.deepseek.com: HTTP 401`; it never contains the key. A 401 or 403 usually means the vendor refused the key; a 404 usually means the address is wrong; a 5xx, `unreachable`, `cut off` or a timeout is on the vendor's side or the network. Check the key and the address, then whether this computer can reach the vendor.

### I have a new computer or a new device. What do I do?

Open the App once on a device that can already open the key; it seals the key again for the new device, and the provider row stops saying **This device cannot open the key yet**. If no such device remains, enter the key again with **Edit** and **Save**.

### What happens to agents if I delete a provider?

The confirmation says: "N agents use it. They keep using what is written on their computers; the App asks you to pick another or clear it." Those agents keep running with the copy on their computer, and the agent's **Provider** row says: "Its model provider was deleted: what is on its computer still works. Pick another, or have it follow its computer again." New agents will not offer the deleted provider. Deleting it in the App does not reach into those computers; pick another provider for the agent, or set it back to following its computer.
