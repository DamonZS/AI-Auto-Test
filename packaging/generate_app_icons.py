"""Convert the supplied company logo to the desktop icon formats."""

import argparse
from pathlib import Path

from PIL import Image, ImageOps


if __name__ == "__main__":
    directory = Path(__file__).resolve().parent
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path, nargs="?", default=directory / "app-icon.png")
    args = parser.parse_args()
    with Image.open(args.source) as source:
        icon = ImageOps.pad(source.convert("RGBA"), (1024, 1024), method=Image.Resampling.LANCZOS)
    icon.save(directory / "app-icon.png", optimize=True)
    icon.save(directory / "app-icon.ico", sizes=[(size, size) for size in (16, 20, 24, 32, 40, 48, 64, 128, 256)])
    icon.save(directory / "app-icon.icns")
