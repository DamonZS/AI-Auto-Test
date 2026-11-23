import argparse
import json
import os
import re
import sys
import time
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, build_opener, ProxyHandler


def prepare(server, browser_url, token_file, desktop_payload=None):
    for attempt in range(60):
        try:
            html = request(server + "/").decode()
            break
        except URLError:
            if attempt == 59:
                raise
            time.sleep(1)
    asset = re.search(r'src="(/assets/[^" ]+\.js)"', html)
    if not asset or not request(server + asset[1]):
        raise RuntimeError("Web is not serving the built Vue application")
    try:
        api(server, "/api/tenants")
    except HTTPError as exc:
        if exc.code != 401:
            raise
    else:
        raise RuntimeError("Production authentication is not enforced")

    if desktop_payload:
        register_in_desktop_ui(server, desktop_payload, activate_worker=token_file is None)
    web_token = login(server)
    tenants = api(server, "/api/tenants", token=web_token)
    if len(tenants) != 1 or tenants[0]["role"] != "owner" or tenants[0]["name"] != "Deployment smoke":
        raise RuntimeError("Smoke requires a fresh database provisioned with only the temporary owner")
    tenant = tenants[0]["id"]
    if token_file is not None:
        service_token = api(server, "/api/auth/service-tokens", "POST", {"scope": "worker"},
                            tenant, web_token)["token"]
        descriptor = os.open(token_file, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        with os.fdopen(descriptor, "w", encoding="utf-8") as stream:
            stream.write(service_token + "\n")
    project = api(server, "/api/projects", "POST", {
        "name": "Deployment smoke", "base_url": browser_url,
    }, tenant, web_token)["id"]
    case = api(server, "/api/cases", "POST", {
        "name": "Rendered Vue page", "project_id": project, "type": "ui",
        "scenario": json.dumps({"steps": [
            {"action": "navigate", "url": browser_url + "/"},
            {"action": "assert_text", "text": "测试平台"},
        ]}, ensure_ascii=False),
    }, tenant, web_token)["id"]
    run = api(server, f"/api/cases/{case}/run", "POST", {}, tenant, web_token)["run_id"]
    print(json.dumps({"tenant": tenant, "run": run}))


def register_in_desktop_ui(server, payload, activate_worker):
    sys.path.insert(0, str(payload / "site"))
    os.environ["PLAYWRIGHT_BROWSERS_PATH"] = str(payload / "browsers")
    from playwright.sync_api import expect, sync_playwright

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        page = browser.new_page()
        page.goto(server)
        page.get_by_label("工作区名称", exact=True).fill("Deployment smoke")
        page.get_by_label("用户名", exact=True).fill(os.environ["PLATFORM_AUTH_USERNAME"])
        page.get_by_label("密码", exact=True).fill(os.environ["PLATFORM_AUTH_PASSWORD"])
        page.get_by_role("button", name="创建工作区", exact=True).click()
        expect(page.get_by_role("button", name="概览", exact=True)).to_be_visible(timeout=15000)
        if activate_worker:
            page.get_by_role("button", name="设置", exact=True).click()
            worker = page.get_by_role("region", name="桌面 Worker", exact=True)
            expect(worker.get_by_role("status")).to_have_text("未激活")
            worker.get_by_role("button", name="激活 Worker", exact=True).click()
            expect(worker.get_by_role("status")).to_have_text("已激活", timeout=15000)
        page.reload()
        expect(page.get_by_role("button", name="登录", exact=True)).to_be_visible(timeout=15000)
        expect(page.get_by_label("工作区名称", exact=True)).to_have_count(0)
        browser.close()


def check(server, tenant, run):
    web_token = login(server)
    for _ in range(90):
        result = api(server, f"/api/runs/{run}", tenant=tenant, token=web_token)
        if result["status"] == "passed":
            print("Web + Go control plane + Python worker + Chromium: passed")
            return
        if result["status"] not in ("queued", "running"):
            break
        time.sleep(1)
    raise RuntimeError(f"Worker run ended in {result['status']}: {result['report']}")


def login(server):
    return api(server, "/api/auth/login", "POST", {
        "username": os.environ["PLATFORM_AUTH_USERNAME"],
        "password": os.environ["PLATFORM_AUTH_PASSWORD"],
    })["token"]


def api(server, path, method="GET", body=None, tenant="", token=""):
    headers = {"Content-Type": "application/json", "X-Tenant-Id": tenant}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    data = json.dumps(body).encode() if body is not None else None
    return json.loads(request(server + path, method, data, headers))


def request(url, method="GET", data=None, headers=None):
    with build_opener(ProxyHandler({})).open(
        Request(url, data, headers or {}, method=method), timeout=5
    ) as response:
        return response.read()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("server")
    commands = parser.add_subparsers(dest="command", required=True)
    setup = commands.add_parser("prepare")
    setup.add_argument("--browser-url")
    setup.add_argument("--worker-token-file", type=Path)
    setup.add_argument("--desktop-payload", type=Path)
    wait = commands.add_parser("check")
    wait.add_argument("tenant")
    wait.add_argument("run")
    args = parser.parse_args()
    if args.command == "prepare":
        if not args.desktop_payload and not args.worker_token_file:
            setup.error("--worker-token-file is required when --desktop-payload is absent")
        prepare(args.server, args.browser_url or args.server, args.worker_token_file, args.desktop_payload)
    else:
        check(args.server, args.tenant, args.run)
