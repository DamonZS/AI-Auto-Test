import asyncio
import sys

from playwright.async_api import async_playwright

TENANT = sys.argv[1]
BASE = sys.argv[2] if len(sys.argv) > 2 else "http://127.0.0.1:8091"
PAGES = [
    ("概览", "overview"),
    ("项目", "projects"),
    ("需求", "requirements"),
    ("版本", "versions"),
    ("环境", "environments"),
    ("测试数据", "test-data"),
    ("用例", "cases"),
    ("执行记录", "runs"),
    ("缺陷", "bugs"),
    ("自愈审批", "patches"),
]


async def main():
    async with async_playwright() as p:
        browser = await p.chromium.launch()
        page = await browser.new_page(viewport={"width": 1280, "height": 860})
        await page.add_init_script(
            f"localStorage.setItem('platform.tenant','{TENANT}')"
        )
        await page.goto(BASE, wait_until="networkidle")
        await page.locator("nav button").first.wait_for(state="visible", timeout=10000)
        await page.wait_for_timeout(1200)
        for index, (label, key) in enumerate(PAGES):
            await page.locator("nav button").nth(index).click()
            await page.wait_for_timeout(700)
            await page.screenshot(path=f"screenshots/{key}.png")
            print(f"{key}: ok")
        await browser.close()


asyncio.run(main())
