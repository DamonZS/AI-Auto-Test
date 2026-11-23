"""Exercise the local Ego browser against a synthetic form."""

import subprocess


SCRIPT = """
const { randomUUID } = await import('node:crypto');
const task = await taskSpace(`Auto-Test Ego smoke ${randomUUID()}`);
try {
    const page = task.page('p1');
    const html = `<label for="name">Name</label><input id="name">
        <button id="submit">Submit</button><p id="status"></p>
        <script>
        document.querySelector('#submit').onclick = () => {
            document.querySelector('#status').textContent = document.querySelector('#name').value;
        };
        </script>`;
    await page.goto('data:text/html,' + encodeURIComponent(html), { timeout: 5000 });
    await page.fill('#name', 'ego-smoke');
    await page.click('#submit', { label: 'Submit smoke form' });
    await page.waitForFunction(() => document.querySelector('#status').textContent === 'ego-smoke', undefined, { timeout: 5000 });
} finally {
    await task.finish({ keep: [] });
}
console.log('Ego smoke passed: form input, click, and rendered result.');
"""


def main() -> None:
    try:
        subprocess.run(["ego-browser", "nodejs"], input=SCRIPT, text=True, check=True, timeout=30)
    except FileNotFoundError:
        raise SystemExit("Ego CLI unavailable: add ego-browser to PATH.") from None
    except subprocess.TimeoutExpired:
        raise SystemExit("Ego smoke timed out: check the local desktop connection.") from None
    except subprocess.CalledProcessError as exc:
        raise SystemExit(exc.returncode) from None


if __name__ == "__main__":
    main()
