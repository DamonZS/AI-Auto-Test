"""本地录制 UI 草稿与离线转换结构化 UI 用例。"""

import argparse
import json
from pathlib import Path
import sys
from zipfile import BadZipFile

from openpyxl.utils.exceptions import InvalidFileException


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)

    record = commands.add_parser("record-ui", help="打开 Chromium 录制结构化 UI 草稿")
    record.add_argument("url", help="测试站点的 HTTP(S) 起始 URL")
    record.add_argument("output", help="草稿 JSON 文件，或 - 输出到 stdout")

    convert = commands.add_parser("convert-recorded", help="录制事件 JSON 数组转场景 JSON")
    convert.add_argument("input", help="事件 JSON 文件，或 - 读取 stdin")
    convert.add_argument("output", nargs="?", default="-", help="场景 JSON 文件，默认 stdout")

    export = commands.add_parser("export-xlsx", help="场景 JSON 转 XLSX 工作簿")
    export.add_argument("input", help="场景 JSON 文件，或 - 读取 stdin")
    export.add_argument("workbook", type=Path, help="输出 XLSX 文件")

    load = commands.add_parser("import-xlsx", help="校验 XLSX 工作簿并导出场景 JSON")
    load.add_argument("workbook", type=Path, help="输入 XLSX 文件")
    load.add_argument("output", nargs="?", default="-", help="场景 JSON 文件，默认 stdout")

    args = parser.parse_args()
    try:
        if args.command == "record-ui":
            from agents.recorder import record_from_terminal

            draft = record_from_terminal(args.url)
            if draft is not None:
                write_json(args.output, draft)
        elif args.command == "convert-recorded":
            events = read_json(args.input)
            if not isinstance(events, list):
                raise ValueError("录制事件 JSON 必须是数组")
            write_json(args.output, convert_recorded_steps(events))
        elif args.command == "export-xlsx":
            export_case_workbook(args.workbook, read_json(args.input))
        else:
            write_json(args.output, import_case_workbook(args.workbook))
    except (ValueError, OSError, BadZipFile, InvalidFileException) as error:
        parser.error(str(error))


def read_json(path):
    data = sys.stdin.buffer.read() if path == "-" else Path(path).read_bytes()
    return json.loads(data.decode("utf-8-sig"))


def write_json(path, scenario):
    data = (json.dumps(scenario, ensure_ascii=False, indent=2, allow_nan=False) + "\n").encode("utf-8")
    if path == "-":
        sys.stdout.buffer.write(data)
    else:
        Path(path).write_bytes(data)


if __name__ == "__main__":
    sys.stderr.reconfigure(encoding="utf-8")
    sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
    from agents.case_workbook import export_case_workbook, import_case_workbook
    from agents.recorded_steps import convert_recorded_steps

    main()
