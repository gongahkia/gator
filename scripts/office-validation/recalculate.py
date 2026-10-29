#!/usr/bin/env python3
"""Verify synthetic Gator XLSX calculations using an installed LibreOffice.

Inputs are never modified; converted workbooks and a JSON report go to a new
output directory. This is an engine compatibility check, not Excel acceptance.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path, PurePosixPath
import shutil
import subprocess
import tempfile
import xml.etree.ElementTree as ET
import zipfile

NS = {"s": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
REL = "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id"


def cells(path):
    result = {}
    with zipfile.ZipFile(path) as archive:
        shared = []
        if "xl/sharedStrings.xml" in archive.namelist():
            shared = ["".join(item.itertext()) for item in ET.fromstring(archive.read("xl/sharedStrings.xml"))]
        relationships = {item.attrib["Id"]: item.attrib["Target"] for item in ET.fromstring(archive.read("xl/_rels/workbook.xml.rels"))}
        workbook = ET.fromstring(archive.read("xl/workbook.xml"))
        for sheet in workbook.findall("s:sheets/s:sheet", NS):
            target = relationships[sheet.attrib[REL]]
            part = target.lstrip("/") if target.startswith("/") else str(PurePosixPath("xl") / target)
            values = {}
            for cell in ET.fromstring(archive.read(part)).findall("s:sheetData/s:row/s:c", NS):
                kind = cell.attrib.get("t", "n")
                raw = cell.findtext("s:v", None, NS)
                value = None
                if kind == "inlineStr":
                    value = "".join(cell.find("s:is", NS).itertext())
                elif raw is not None and raw != "":
                    if kind == "s":
                        value = shared[int(raw)]
                    elif kind == "b":
                        value = raw == "1"
                    elif kind == "e":
                        value = {"error": raw}
                    elif kind == "n":
                        value = float(raw)
                    else:
                        value = raw
                values[cell.attrib["r"]] = {"formula": cell.find("s:f", NS) is not None, "value": value}
            result[sheet.attrib["name"]] = values
    return result


def check(expected, actual, label, formula, uncalculated=False):
    count = 0
    for sheet, addresses in expected.items():
        for address, value in addresses.items():
            where = f"{label}:{sheet}!{address}"
            cell = actual.get(sheet, {}).get(address)
            if cell is None or cell["formula"] != formula:
                raise AssertionError(f"{where}: missing cell or wrong formula/literal type")
            got = cell["value"]
            if uncalculated:
                if got is not None:
                    raise AssertionError(f"{where}: unverified formula cache was retained: {got!r}")
            elif type(value) in (int, float):
                if type(got) not in (int, float) or not math.isfinite(got) or not math.isclose(got, value, rel_tol=1e-12, abs_tol=1e-9):
                    raise AssertionError(f"{where}: expected {value!r}, calculated {got!r}")
            elif type(got) is not type(value) or got != value:
                raise AssertionError(f"{where}: expected {value!r}, calculated {got!r}")
            count += 1
    return count


def verify_outputs(expectations, directory):
    formulas = literals = 0
    for name, expected in expectations.items():
        actual = cells(directory / name)
        formulas += check(expected["formulas"], actual, name, True)
        literals += check(expected.get("values", {}), actual, name, False)
    return formulas, literals


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("fixtures", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--soffice", default="libreoffice")
    args = parser.parse_args()
    fixtures, output = args.fixtures.resolve(), args.output.resolve()
    engine = shutil.which(args.soffice)
    if not engine:
        parser.error("Install LibreOffice Calc or pass --soffice /path/to/soffice.")
    expectations = json.loads((fixtures / "calculation-expectations.json").read_text())
    if not expectations:
        raise AssertionError("No workbook expectations supplied")
    if set(expectations) != {path.name for path in fixtures.glob("*.xlsx")}:
        raise AssertionError("Every generated XLSX must have calculation expectations; use a complete, dedicated fixture directory")
    inputs = []
    digests = {}
    for name, expected in expectations.items():
        if Path(name).name != name or not name.endswith(".xlsx"):
            raise AssertionError("Expectations must name XLSX files directly inside the fixture directory")
        path = fixtures / name
        actual = cells(path)
        expected_formulas = {(sheet, address) for sheet, addresses in expected["formulas"].items() for address in addresses}
        actual_formulas = {(sheet, address) for sheet, addresses in actual.items() for address, cell in addresses.items() if cell["formula"]}
        if expected_formulas != actual_formulas:
            raise AssertionError(f"{name}: every formula must have an independent expected value")
        check(expected["formulas"], actual, name, True, uncalculated=True)
        check(expected.get("values", {}), actual, name, False)
        digests[name] = hashlib.sha256(path.read_bytes()).hexdigest()
        inputs.append(str(path))
    # Refuse reuse so an engine failure cannot pass against stale output files.
    output.mkdir(parents=True, exist_ok=False)
    version = subprocess.run([engine, "--version"], check=True, capture_output=True, text=True, timeout=20).stdout.strip()
    with tempfile.TemporaryDirectory(prefix="gator-calc-profile-") as temporary:
        result = subprocess.run([engine, "-env:UserInstallation=" + Path(temporary).as_uri(), "--headless", "--norestore", "--convert-to", "xlsx:Calc MS Excel 2007 XML", "--outdir", str(output), *inputs], capture_output=True, text=True, timeout=120)
    print(result.stdout, end="")
    if result.returncode:
        raise RuntimeError(f"LibreOffice failed ({result.returncode}): {result.stderr}")
    formulas, literals = verify_outputs(expectations, output)
    for name, before in digests.items():
        if hashlib.sha256((fixtures / name).read_bytes()).hexdigest() != before:
            raise AssertionError(f"Input workbook changed: {name}")
    # Negative control: a wrong expected total must fail against real engine output.
    wrong = json.loads(json.dumps(expectations))
    wrong["professional-new.xlsx"]["formulas"]["Summary"]["B4"] = 999
    try:
        verify_outputs(wrong, output)
    except AssertionError:
        pass
    else:
        raise AssertionError("Verifier accepted an intentionally wrong total")
    report = {"engine": version, "workbooks": len(inputs), "formula_values": formulas, "literal_values": literals, "input_sha256": digests, "negative_control": "rejected wrong total", "scope": "Synthetic calculation compatibility; not Microsoft Excel or general workbook fidelity acceptance."}
    (output / "calculation-report.json").write_text(json.dumps(report, indent=2) + "\n")
    print(f"PASS: {len(inputs)} workbooks, {formulas} calculated formula values, {literals} literal values; originals unchanged; wrong-total control rejected. Engine: {version}")


if __name__ == "__main__":
    main()
