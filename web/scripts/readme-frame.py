# readme-frame.py <in.png> <out.webp>: a macOS window and shadow around a 2x
# app screenshot from readme-shots.cjs, scaled to 3/4, as in docs/assets/app.
import sys
from PIL import Image, ImageDraw, ImageFilter
src, dst = sys.argv[1], sys.argv[2]
im = Image.open(src).convert('RGBA'); w, h = im.size
r = 24  # corner radius (2x)
mask = Image.new('L', (w, h), 0); ImageDraw.Draw(mask).rounded_rectangle([0, 0, w - 1, h - 1], r, fill=255)
win = Image.new('RGBA', (w, h)); win.paste(im, (0, 0), mask)
d = ImageDraw.Draw(win)
for i, c in enumerate(['#ff5f57', '#febc2e', '#28c840']):  # traffic lights
    x, y = 40 + i * 40, 36
    d.ellipse([x - 12, y - 12, x + 12, y + 12], fill=c)
dark = sum(im.getpixel((w // 2, h - 4))[:3]) < 200
d.rounded_rectangle([0, 0, w - 1, h - 1], r, outline=(255, 255, 255, 40) if dark else (0, 0, 0, 40), width=2)
pad = 80
canvas = Image.new('RGBA', (w + pad * 2, h + pad * 2), (0, 0, 0, 0))
sh = Image.new('RGBA', canvas.size, (0, 0, 0, 0))
ImageDraw.Draw(sh).rounded_rectangle([pad, pad + 20, pad + w, pad + h + 20], r, fill=(0, 0, 0, 90))
canvas = Image.alpha_composite(canvas, sh.filter(ImageFilter.GaussianBlur(30)))
canvas.alpha_composite(win, (pad, pad))
canvas = canvas.resize((canvas.width * 3 // 4, canvas.height * 3 // 4), Image.LANCZOS)

canvas.save(dst, "WEBP", quality=92, method=6)
