# Voice

When you would rather talk than type, you can. myco has two kinds of voice: **dictation** writes what you say into the box, for you to check before sending; **voice chat** is a live conversation with an agent, like a call, where what you ask gets done straight away.

## Dictation

The microphone at the bottom right of the box, left of the send button, is dictation. It is there whether or not the box has text.

1. Click the microphone. It turns red and pulses gently while it listens.
2. Speak. Your words go into the box, after anything you already typed. Pauses do not cut it off; keep talking and it keeps writing.
3. It stops when you finish, or click it again. Typing or sending stops it too.
4. Check it, fix anything, press Enter.

![Dictation: the microphone turns red and your words go into the box](shot:dictation)

It listens in the App's language, not the system's: a Chinese App listens for Chinese, an English one for English. To change it, change the App's language under **Settings › Appearance**.

**Hold to talk**: with the caret in the box, hold **⌥Space** (Mac) or **⌃M** and speak; let go to stop.

### On a Mac

The App uses Apple's speech recognition (on the device when it can). The first time, the system asks for two permissions, Microphone and Speech Recognition; allow both.

If you said no, the microphone turns amber, and hovering it tells you where to turn them on: **System Settings › Privacy & Security › Microphone** (or **Speech Recognition**), then switch on myco.

### On Windows

When you click the microphone, the App first switches this window's input language to the App's language, then opens Windows' own voice typing (the one Win+H opens), and the system types into the box.

Hold to talk is **Ctrl+M** on Windows (⌥Space would open the window menu there).

### No microphone?

When this computer has no speech recognition the App can use (in a browser that does not support it, for example), the microphone is not shown. Terminals have no dictation.

## Voice chat

Voice chat is like a call with this agent itself (as in ChatGPT and Claude: one chat, one call). It is the agent who answers, saying "I'll fix that" or "let me check": on the call it speaks through a quick model, while the real work happens in its own session.

- once connected, it says hello first, mentioning any work in progress or a question it is waiting on;
- ask how things are going and it answers;
- ask for something to be done and it takes it on: as an addition if it is busy, as a new task if it is free, just as pressing Enter would, with no extra confirmation;
- when it is done or has a question for you, it tells you in a sentence or two at a quiet moment, or says the rest is in the chat when it is long;
- start talking while it speaks and it stops to listen.

Voice chat is in the **Mac App** and the phone app for now.

### Setting it up

Open **Settings › Voice**:

1. **The voice's model**: fill in **Address**, **Model** and **Key**. Any OpenAI-compatible service works (OpenAI, DeepSeek, Doubao, OpenRouter…); pick a quick one: it only does the talking, while the agent does the hard work in its own session. Left empty, the one set up under Translation is used.
2. **Speech**: the service that says the agent's words out loud on the call. For now that is **Doubao voice**: in the Volcano Engine console, under Doubao voice, turn on speech synthesis, create an API key and paste it here.
3. Choose a **Voice** (grouped by language) and **Speed**, and click **Try it** to hear a sentence. For a cloned voice, choose "Other (type a voice ID)".
4. Click **Save**.

**Words to hear right**: voice chat and dictation listen for these words, and fix them when they are heard as something else. The names of agents, projects and members, and common words such as Claude, Codex, GitHub and PR, are already included; add others one per line. For a word often misheard, write `Claude = clod, cloud`: what follows the equals sign is how it tends to be heard, and that is changed back to the word before it. Only these words are changed; nothing else you say is touched.

![Settings › Voice: the model and the speech service](shot:settings-voice)

Keys are kept in this computer's system keychain; the page only says "Kept in the system keychain (not shown)".

At the bottom, **On your other devices** is on by default: these settings, keys included, are kept on the workspaces you are connected to, so your phone and other computers need no setup. Note that the administrator of the computer a workspace runs on can read its data; to keep your keys on this computer only, turn it off.

### Making a call

Once set up, the round button on the right of a chat's box becomes **Voice chat** (a waveform) when **the box is empty and the agent is not working**.

1. Click the waveform to connect.
2. A voice bar appears above the box. The small dot on the left shows its state (Listening, Thinking, Speaking, Muted), next to "Voice chat with …".
3. Captions run below: your lines start with "You", its lines with its small face (it is the agent itself talking). What it takes on shows as a small line with an arrow, such as "→ On it: check the download links".
4. Two small round buttons on the right: **Mute** and the red **Hang up**. **Esc** hangs up too.

What you hand the agent by voice shows in the chat as usual, with a small "By voice" tag above it; hover to see your words as you said them.

![The voice bar: its state, captions, and what went to the agent](shot:voice-chat)

The box on a project's home works the same way: the waveform opens the chat with the recipient and connects.

### Talking while you move around

- Going to another chat, a project or Settings does not end the call. A small pill appears at the top right with who you are talking to and whether it is listening or speaking, plus mute and hang up; click the name to go back to that chat.
- One call at a time. Voice chat in another chat hangs up the first; the button's tooltip says "Hang up on A and voice chat with B".

![The call tucked into a pill at the top right](shot:voice-chip)

## Common questions

**Why is there no waveform button?** Check that this is the Mac App; that both the model and speech are set up under **Settings › Voice**; that the box is empty; and that the agent is not working (then the round button is Stop). Terminal chats have no voice chat.

**Are recordings kept?** No. The small talk in the voice bar stays on this computer and recordings are not kept; what you hand the agent stays in the chat as an ordinary message.

**What if it mishears and sets off the wrong work?** What it takes on is written in the captions, and the message in the chat shows your words as said. If it is wrong, add a correction or stop it in the chat right away; see [Chatting with agents](#help/chat).
