"""Wait for a page to expose stable, usable controls."""

import time


async def wait_for_ready(page, timeout_ms=18_000, stable_ms=600):
    """Wait for visible controls to become idle and remain stable.

    Results include status, timeout reason, usable control count, busy state,
    elapsed time, and the duration of the stable control signature.
    """
    started = time.monotonic()
    stable_since = started
    previous_signature = None
    last_controls = 0
    last_busy = False
    observed_controls = False

    while (time.monotonic() - started) * 1000 < timeout_ms:
        state = await page.evaluate("""() => {
            const controls = [...document.querySelectorAll(
                'button,input:not([type=hidden]),select,textarea,a[href],'
                + '[role=button],[role=tab],[role=combobox],[role=menuitem]'
            )].filter(element => {
                const rect = element.getBoundingClientRect();
                const style = getComputedStyle(element);
                return rect.width > 0 && rect.height > 0
                    && style.display !== 'none' && style.visibility !== 'hidden'
                    && !element.closest('[aria-hidden="true"], [inert]');
            });
            const usableControls = controls.filter(element =>
                !element.disabled
                && element.getAttribute('aria-disabled') !== 'true'
            );
            const busy = [...document.querySelectorAll(
                '[aria-busy="true"], .ant-spin-spinning, .el-loading-mask, [role="progressbar"]'
            )].some(element => {
                const rect = element.getBoundingClientRect();
                const style = getComputedStyle(element);
                return rect.width > 0 && rect.height > 0
                    && style.display !== 'none' && style.visibility !== 'hidden';
            }) || document.body?.innerText.split('\\n').some(line =>
                /^(?:loading\\.{0,3}|加载中[.。…]*|正在加载[.。…]*)$/i.test(line.trim())
            ) || false;
            const signature = controls.slice(0, 60).map(element => [
                element.tagName,
                element.getAttribute('type'),
                element.getAttribute('role'),
                element.getAttribute('name'),
                element.getAttribute('placeholder'),
                element.getAttribute('aria-label'),
                element.textContent?.trim().slice(0, 25),
                element.disabled,
                element.getAttribute('aria-disabled'),
                getComputedStyle(element).visibility,
            ].join(':')).join('|');
            return {controls: usableControls.length, busy, signature};
        }""")

        now = time.monotonic()
        last_controls = state["controls"]
        last_busy = state["busy"]
        observed_controls = observed_controls or last_controls > 0
        if state["signature"] != previous_signature or last_busy or last_controls == 0:
            previous_signature = state["signature"]
            stable_since = now

        stable_for_ms = int((now - stable_since) * 1000)
        if last_controls > 0 and not last_busy and stable_for_ms >= stable_ms:
            return {
                "status": "ready",
                "reason": None,
                "controls": last_controls,
                "busy": False,
                "duration_ms": int((now - started) * 1000),
                "stable_ms": stable_for_ms,
            }

        remaining_ms = timeout_ms - (now - started) * 1000
        if remaining_ms > 0:
            await page.wait_for_timeout(min(50, max(1, stable_ms // 4), remaining_ms))

    reason = "loading" if last_busy else "unstable" if observed_controls else "blank"
    return {
        "status": "timeout",
        "reason": reason,
        "controls": last_controls,
        "busy": last_busy,
        "duration_ms": int((time.monotonic() - started) * 1000),
        "stable_ms": int((time.monotonic() - stable_since) * 1000),
    }
