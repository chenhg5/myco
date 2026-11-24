# AIGC Canvas

The shared `nodes` collection contains one record per workflow node. A record has `kind` (note, text, image, video), `title`, `prompt` (also the body of notes), `model`, `options` (optional JSON object encoded as text), `from` (upstream record ID or empty), `x`, `y` (canvas units), and `output` (generated text or an HTTPS asset URL). Keep connections acyclic. Read the current version before writing, and preserve the other fields.

Right-click or double-click the empty canvas to add a node at the cursor. Drag a node’s output port to an input or directly to the destination card to connect it, or release on the blank canvas to create a connected node. Dragging a card saves its position after release.

The user can run a node and its full upstream chain in the app. Notes pass through their content. Text uses OpenRouter, and images and video use Replicate. Model inputs depend on the selected model. A JSON string value of `{{upstream}}` is replaced with the upstream output; otherwise the output is appended to the prompt. The workspace shares resulting node content and links. Replicate output links can expire, so use the original file link to save a copy promptly.

Keep API keys outside records and chat. The app accepts user keys for the current browser session. Agents can create and edit node records through page data, but generation from this UI uses the keys entered by the person.
