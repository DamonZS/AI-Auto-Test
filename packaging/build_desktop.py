import argparse
import hashlib
import os
import platform
import shutil
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path
from urllib.request import urlretrieve


ROOT = Path(__file__).resolve().parents[1]
RELEASE = "20261110"
PYSIDE_VERSION = "6.10.2"
RUNTIMES = {
    ("Windows", "AMD64"): ("x86_64-pc-windows-msvc", "2e0fed09d48989f64c9305da9ff2d37a41c0d13ab2aa6283405b2b620dfbf805"),
    ("Darwin", "arm64"): ("aarch64-apple-darwin", "7a2384641584ab9984221fcb0fc9265df465922af71be71410529e2a9c3f0540"),
}


def run(*command, **options):
    subprocess.run(list(map(str, command)), check=True, **options)


def stage(payload, scratch, windows):
    payload.mkdir(parents=True)
    web = scratch / "web"
    web.mkdir()
    web_source = ROOT / "platform/web"
    for name in ("package.json", "package-lock.json", "index.html", "vite.config.js"):
        shutil.copy2(web_source / name, web)
    for source in (web_source / "src").rglob("*"):
        if source.is_file() and source.suffix in {".js", ".vue", ".css"}:
            destination = web / source.relative_to(web_source)
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, destination)
    npm = (os.environ["ComSpec"], "/D", "/C", "npm.cmd") if windows else ("npm",)
    run(*npm, "ci", cwd=web)
    run(*npm, "run", "build", "--", "--outDir", payload / "web", cwd=web)
    run("go", "build", "-trimpath", "-buildvcs=false", "-o",
        payload / ("autotest-server.exe" if windows else "autotest-server"), "./cmd/server",
        cwd=ROOT / "platform/server", env={**os.environ, "CGO_ENABLED": "0"})

    target, digest = RUNTIMES[platform.system(), platform.machine()]
    archive = scratch / "runtime.tar.gz"
    name = f"cpython-3.13.18%2B{RELEASE}-{target}-install_only_stripped.tar.gz"
    urlretrieve(f"https://github.com/astral-sh/python-build-standalone/releases/download/{RELEASE}/{name}", archive)
    with archive.open("rb") as source:
        if hashlib.file_digest(source, "sha256").hexdigest() != digest:
            raise RuntimeError("Bundled Python checksum mismatch")
    with tarfile.open(archive, "r:gz") as source:
        source.extractall(payload, filter="data")

    site = payload / "site"
    run(sys.executable, "-m", "pip", "install", "--only-binary=:all:", "--no-compile", "--target", site,
        "-r", ROOT / "deploy/worker-requirements.txt", f"PySide6=={PYSIDE_VERSION}")
    python = payload / ("python/python.exe" if windows else "python/bin/python3")
    run(python, "-m", "playwright", "install", "--only-shell", "chromium", env={
        **os.environ, "PYTHONPATH": str(site), "PLAYWRIGHT_BROWSERS_PATH": str(payload / "browsers"),
    })
    for package in ("agents", "auto_ai"):
        (payload / package).mkdir()
        for source in (ROOT / package).glob("*.py"):
            shutil.copy2(source, payload / package / source.name)
    bundle_icon = "app-icon.ico" if windows else "app-icon.icns"
    for name in ("desktop.py", "desktop_window.py", "app-icon.png", bundle_icon):
        shutil.copy2(ROOT / "packaging" / name, payload)


def build(output, iscc):
    host = (platform.system(), platform.machine())
    if host not in RUNTIMES or sys.version_info[:2] != (3, 13):
        raise RuntimeError("Build requires Windows x64 or macOS arm64 with Python 3.13")
    windows = host[0] == "Windows"
    if windows and not iscc:
        raise RuntimeError("Provide the verified Inno Setup compiler with --iscc")
    if not (ROOT / "agents/worker_credentials.py").is_file():
        raise RuntimeError("Desktop packaging requires the production authentication changes")
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="autotest-build-") as temporary:
        scratch = Path(temporary)
        disk = scratch / "disk"
        contents = disk / "Auto-Test.app/Contents"
        payload = scratch / "payload" if windows else contents / "Resources"
        stage(payload, scratch, windows)
        if windows:
            run(iscc, "/Qp", f"/DPayload={payload}", f"/O{output}", ROOT / "packaging/windows.iss")
        else:
            (contents / "MacOS").mkdir()
            launcher = contents / "MacOS/Auto-Test"
            shutil.copyfile(ROOT / "packaging/macos-launcher", launcher)
            launcher.chmod(0o755)
            shutil.copyfile(ROOT / "packaging/macos.plist", contents / "Info.plist")
            (disk / "Applications").symlink_to("/Applications")
            run("hdiutil", "create", "-ov", "-fs", "HFS+", "-format", "UDZO", "-volname", "Auto-Test",
                "-srcfolder", disk, output / "Auto-Test-macos-arm64.dmg")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--iscc", type=Path)
    args = parser.parse_args()
    build(args.output.resolve(), args.iscc)
