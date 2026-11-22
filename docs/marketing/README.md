# Marketing images

Seven images in English and Chinese, at Product Hunt's gallery size (1270×760, saved at 2×: 2540×1520), for launches and sharing: [`../assets/marketing/en`](../assets/marketing/en) and [`../assets/marketing/zh`](../assets/marketing/zh).

| # | Headline (English) | Shows |
|---|---|---|
| 01 | All your AI agents, one app. | A project: the chat with its coordinator, members beside it |
| 02 | Agents that work together | Four agents side by side |
| 03 | Your computers, in the loop | A server's terminal next to the project chat |
| 04 | See the work, not just the answer | One conversation, steps folded into a line |
| 05 | Apps your agents use too | The apps library |
| 06 | Work that starts on its own | Automations and their templates |
| 07 | Your agents, in your pocket | The iPhone app (its built-in demo workspace, in Chinese) |

## Making them again

1. Capture the App from the showcase mock, light, in each language (start the mock afresh for each):

   ```bash
   (cd web && MOCK_SHELL=mac MOCK_SHOWCASE=en MOCK_UPDATE=none node mock/app-server.mjs & npx vite --port 5174)
   cd docs/promo-video && SCHEME=light node capture.cjs en <captures>/en
   ```

   Then the same with `zh`.
2. Phone pictures: the iPhone app in a simulator with `-myco-start demo` and `-myco-route tab:chats` or `-myco-route agent:cloud-lead`, with `-AppleLanguages "(en)"` or `"(zh-Hans)"`; save them as `<phones>/<en|zh-Hans>-<chats|agent>.jpg` (640 px wide is enough).
3. Compose: `node docs/marketing/compose.cjs <captures> <phones> <work dir>` writes PNGs to `<work dir>/out/<lang>/`. The headlines and lines are in the script.
