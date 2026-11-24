# Media Kit · 素材库

Shared release entries live in page data. Original screenshots live in the page's project files. Use the project name and page ID from the request or page metadata.

## Find and read

Set `PAGE` to the page ID and `PROJECT` to the project name. List entries and filter locally. Change the tag, status or keyword in this example as needed.

```sh
myco page data list --page "$PAGE" --collection entries |
  jq '.records[] | select((.data.tags // []) | index("App")) |
      select(.data.status == "ready") |
      select([.data.title, .data.pitch_zh, .data.pitch_en, .data.copy_zh, .data.copy_en] |
             map(. // "") | join(" ") | ascii_downcase | contains("token"))'
myco page data get --page "$PAGE" --collection entries --id "$ENTRY"
myco page data list --page "$PAGE" --collection images |
  jq --arg entry "$ENTRY" '[.records[] | select(.data.entry == $entry)] | sort_by(.data.order)'
myco project file get --name "$PROJECT" --file "$FILENAME" --out ./screenshot.png
```

Use each image record's `data.file` for `FILENAME`. Sample images only have previews.

## Create and edit an entry

```sh
myco page data put --page "$PAGE" --collection entries --id "$ENTRY" --if-version 0 --file entry.json
```

Choose a unique entry ID of 1–42 lowercase letters, digits, underscores or hyphens. A new entry JSON object includes `title`, ISO `date`, `pitch_zh`, `pitch_en`, an array `tags`, and `status` (`draft`, `ready` or `published`). Optional fields are `version`, `copy_zh`, `copy_en`, and `channels` (where it was published). To edit, read the existing record, merge the desired changes into its `data`, and use `put` with the returned version as `--if-version`. On conflict read and merge again.

## Add a screenshot

1. Choose a unique lowercase alphanumeric suffix of 8–32 characters. Form the file name `mk-${PAGE}-${ENTRY}-${SUFFIX}.png` (or `.jpg` / `.webp`). Keep the entry ID in the file name.
2. Upload the original: `myco project file put --name "$PROJECT" --path ./screenshot.png --as "$FILENAME"`.
3. Create an `images` record with `myco page data put --page "$PAGE" --collection images --id "img_$SUFFIX" --if-version 0 --file image.json`. Include `entry` (entry ID), `file` (project file name), `name` (display name), `language` (`zh`, `en`, `none`), `theme` (`light`, `dark`), `width` and `height` (pixels), `bytes`, and `order` (ascending, first image is the cover). Optionally include `caption` and a PNG, JPEG or WebP data URI `preview` of up to 18,000 characters for the gallery. If record creation fails after upload, use the same file name to finish the record.

The whole project has 100 file slots shared with its documents. Each upload is capped at 20 MiB. Project membership controls access to the originals. The desktop app needs the project image bridge to upload, download or copy originals. Agents use the CLI.
