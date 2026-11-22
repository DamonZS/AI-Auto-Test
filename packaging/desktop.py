import argparse
import os
import signal
import subprocess
import sys
import time
import webbrowser
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import ProxyHandler, build_opener


PAYLOAD = Path(__file__).resolve().parent
WINDOWS = sys.platform == "win32"


def lock(handle):
    try:
        if WINDOWS:
            import msvcrt
            msvcrt.locking(handle.fileno(), msvcrt.LK_NBLCK, 1)
        else:
            import fcntl
            fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
        return True
    except OSError:
        return False


def worker_token(data):
    configured = os.environ.get("PLATFORM_WORKER_TOKEN", "").strip()
    if configured:
        return configured
    token_file = data / "worker-token.txt"
    return token_file.read_text(encoding="utf-8-sig").strip() if token_file.is_file() else ""


def wait_for_control(control, stop_file, server):
    opener = build_opener(ProxyHandler({}))
    for _ in range(30):
        if stop_file.exists():
            return False
        if control.poll() is not None:
            raise RuntimeError("Control plane failed to start")
        try:
            with opener.open(server + "/api/tenants", timeout=2):
                raise RuntimeError("Control plane did not require authentication")
        except HTTPError as exc:
            if exc.code != 401:
                raise
            return True
        except URLError:
            time.sleep(1)
    raise RuntimeError("Control plane did not become ready")


def serve(args, stop_file, log):
    server = f"http://127.0.0.1:{args.port}"
    env = {**os.environ, "PYTHONPATH": os.pathsep.join((str(PAYLOAD), str(PAYLOAD / "site"))),
           "PYTHON_DOTENV_DISABLED": "1",
           "PYTHONDONTWRITEBYTECODE": "1", "LANGSMITH_TRACING": "false", "NEO4J_ENABLED": "false",
           "PLAYWRIGHT_BROWSERS_PATH": str(PAYLOAD / "browsers"), "PYTHONNOUSERSITE": "1"}
    for credential in ("PLATFORM_AUTH_USERNAME", "PLATFORM_AUTH_PASSWORD", "PLATFORM_WORKER_TOKEN"):
        env.pop(credential, None)
    options = {"cwd": args.data_dir, "env": env, "stdout": log, "stderr": log,
               "creationflags": subprocess.CREATE_NO_WINDOW if WINDOWS else 0}
    binary = PAYLOAD / ("autotest-server.exe" if WINDOWS else "autotest-server")
    control = subprocess.Popen([binary, "-addr", f"127.0.0.1:{args.port}", "-db",
                                "platform.db", "-web", PAYLOAD / "web"], **options)
    worker = None
    try:
        if not wait_for_control(control, stop_file, server):
            return
        if not args.no_browser:
            webbrowser.open(server)
        print("Waiting for a worker service token", file=log, flush=True)
        while not stop_file.exists() and control.poll() is None:
            if worker is None or worker.poll() is not None:
                token = worker_token(args.data_dir)
                if token:
                    python = PAYLOAD / ("python/python.exe" if WINDOWS else "python/bin/python3")
                    worker_options = {**options, "env": {
                        **env, "PLATFORM_WORKER_TOKEN": token,
                        "PLATFORM_WORKER_CACHE_DIR": str(args.data_dir / "cache"),
                    }}
                    worker = subprocess.Popen([python, "-m", "agents.worker_credentials", "--server", server],
                                              **worker_options)
            time.sleep(2)
    finally:
        for process in (worker, control):
            if process and process.poll() is None:
                process.terminate()
                process.wait()


def main():
    base = Path(os.environ["LOCALAPPDATA"]) if WINDOWS else Path.home() / "Library/Application Support"
    parser = argparse.ArgumentParser()
    parser.add_argument("--data-dir", type=Path, default=base / "Auto-Test")
    parser.add_argument("--port", type=int, default=8090)
    parser.add_argument("--no-browser", action="store_true")
    parser.add_argument("--stop", action="store_true")
    args = parser.parse_args()
    args.data_dir = args.data_dir.resolve()
    if args.stop and not args.data_dir.is_dir():
        return
    args.data_dir.mkdir(parents=True, exist_ok=True)
    stop_file = args.data_dir / "stop.request"
    with (args.data_dir / "desktop.lock").open("a+b") as handle:
        if args.stop:
            if not lock(handle):
                stop_file.touch()
                for _ in range(60):
                    time.sleep(0.5)
                    if lock(handle):
                        return
                raise RuntimeError("Desktop did not stop")
            return
        if not lock(handle):
            if not args.no_browser:
                webbrowser.open(f"http://127.0.0.1:{args.port}")
            return
        stop_file.unlink(missing_ok=True)
        signal.signal(signal.SIGTERM, lambda *_: stop_file.touch())
        with (args.data_dir / "desktop.log").open("a", encoding="utf-8") as log:
            try:
                serve(args, stop_file, log)
            except Exception as exc:
                print(str(exc), file=log, flush=True)
                raise


if __name__ == "__main__":
    main()
