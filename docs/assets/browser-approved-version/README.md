# Owner-approved browser scripts

Four screenshots at 2× scale (2880 × 2000), using synthetic data only. Chinese
and English are independent sets in light and dark themes. These show the App's
trusted approval dialog, not a production website or a real browser session.

Reproduce with the existing mock screenshot runner:

```sh
APPS_TEST_ROOT=/path/to/apps node web/scripts/media-kit-shots.mjs \
  --scene browser-approved-version --lang zh,en --out /tmp/browser-media
```

The native backend generates the review from the exact immutable snapshot.
Screenshots do not replace the local policy, fake component and Hub tests.
