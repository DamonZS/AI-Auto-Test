"""Regenerate the checked-in desktop icons with Pillow."""

from pathlib import Path

from PIL import Image, ImageDraw, ImageOps


def draw_icon():
    scale = 16
    side = 128 * scale
    mask = Image.new("L", (side, side))
    ImageDraw.Draw(mask).rounded_rectangle((4 * scale, 4 * scale, 124 * scale, 124 * scale), 29 * scale, fill=255)
    icon = ImageOps.colorize(Image.linear_gradient("L"), "#2B2459", "#100F2A").resize(mask.size).convert("RGBA")
    icon.putalpha(mask)
    draw = ImageDraw.Draw(icon)
    draw.rounded_rectangle((5 * scale, 5 * scale, 123 * scale, 123 * scale), 28 * scale,
                           outline="#484179", width=scale)
    stroke(draw, [(72, 33), (40, 33), (32, 41), (32, 87), (40, 95), (86, 95), (94, 87), (94, 70)],
           5, "#8B83CA", scale)
    stroke(draw, [(49, 62), (64, 77), (99, 42)], 12, "#79F4BA", scale)
    return icon.resize((1024, 1024), Image.Resampling.LANCZOS)


def stroke(draw, points, width, color, scale):
    pixels = [(horizontal * scale, vertical * scale) for horizontal, vertical in points]
    diameter = width * scale
    radius = diameter // 2
    draw.line(pixels, fill=color, width=diameter, joint="curve")
    for horizontal, vertical in (pixels[0], pixels[-1]):
        draw.ellipse((horizontal - radius, vertical - radius, horizontal + radius, vertical + radius), fill=color)


if __name__ == "__main__":
    directory = Path(__file__).resolve().parent
    icon = draw_icon()
    icon.resize((512, 512), Image.Resampling.LANCZOS).save(directory / "app-icon.png", optimize=True)
    icon.save(directory / "app-icon.ico", sizes=[(size, size) for size in (16, 20, 24, 32, 40, 48, 64, 128, 256)])
    icon.save(directory / "app-icon.icns")
