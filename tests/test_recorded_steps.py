import pytest

from agents.recorded_steps import convert_recorded_steps


def test_convert_recorded_steps_preserves_order_and_only_exports_data_reference():
    events = [
        {"action": "goto", "url": "https://qa.example/form"},
        {
            "action": "fill",
            "selector": 'role=textbox[name="Account"]',
            "data_key": "account",
            "value": "synthetic-input",
        },
        {"action": "scroll", "enabled": False},
        {"action": "click", "selector": r'role=button[name="Submit \"draft\""]'},
    ]

    assert convert_recorded_steps(events) == {
        "steps": [
            {"action": "navigate", "url": "https://qa.example/form"},
            {"action": "fill", "role": "textbox", "name": "Account", "data_key": "account"},
            {"action": "click", "role": "button", "name": 'Submit "draft"'},
        ]
    }


def test_accepts_platform_navigation_action():
    event = {"action": "navigate", "url": "https://qa.example/"}
    assert convert_recorded_steps([event]) == {"steps": [event]}


@pytest.mark.parametrize(
    "event",
    [
        {"action": "goto", "url": " "},
        {"action": "click", "selector": "#submit"},
        {"action": "click", "selector": 'role=button[name=""]'},
        {"action": "click", "selector": 'role=button[name="Submit"]', "button": "right"},
        {"action": "fill", "selector": 'role=textbox[name="Account"]', "value": "synthetic-input"},
        {"action": "scroll"},
    ],
)
def test_rejects_events_that_cannot_be_replayed(event):
    with pytest.raises(ValueError, match=r"第 1 条") as error:
        convert_recorded_steps([event])
    assert "synthetic-input" not in str(error.value)


def test_rejects_empty_scenario():
    with pytest.raises(ValueError, match="至少"):
        convert_recorded_steps([{"action": "click", "enabled": False}])
