#!/usr/bin/env python3
"""Render synthetic Word/PowerPoint fixtures and check their PDF layout.

Requires LibreOffice Writer/Impress and Poppler. Outputs are independent-engine
evidence, not native Microsoft Office acceptance or general typography approval.
"""
import argparse
import copy
import hashlib
import json
import math
from pathlib import Path
import shutil
import subprocess
import tempfile
import unicodedata
import xml.etree.ElementTree as ET

XHTML = "{http://www.w3.org/1999/xhtml}"


def normalize(text):
    # Poppler adds bidirectional formatting controls to logical-order RTL text.
    return " ".join("".join(c for c in text if unicodedata.category(c) != "Cf").split())


def expectations():
    multilingual = ["多语言", "العربية", "客户服务 — 质量与下一步", "مراجعة الخدمة والخطوات التالية"]
    first = ["Results & decisions", "Availability improved to 99.7%.", "One priority-one ticket remains open.", "Next: complete the regional rollout."]
    regional = ["Regional rollout", "Twelve regions are live.", "The final onboarding review is due next week."]
    edited = [["Follow-up", "Review the completed rollout."], ["Updated results & decisions", "Availability remains 99.7%.", "The final ticket is resolved."], multilingual]
    host = ["Edited through the shared file editor.", "Second line Summary 🐊"]
    document = ["October service review", "Results & decisions", "Availability improved to 99.7%.", "The final ticket remains open.", "Next review: Friday.", "Keep the unresolved issue visible.", "Twelve regions live", "One region pending", "1. Review the ticket", "2. Complete the rollout", "Region", "Availability", "Status", "APAC", "99.7%", "Open", "Due Friday", "EMEA", "99.9%", "Complete"]
    next_page = ["Next review", "Customer review — 客户服务 🐊"]
    result = {}
    for theme in ("professional", "minimal", "report"):
        result[theme + "-new.pptx"] = [first, regional, multilingual]
        result[theme + "-edited.pptx"] = edited
        result[theme + "-new.docx"] = [document + ["Service operations", "Internal review"], next_page + ["Service operations", "Internal review"]]
        result[theme + "-filled.docx"] = [document + ["Retained template header", "Before inserted body"], next_page + ["Retained template header", "After inserted body"]]
    result["rich-template.pptx"] = [first, regional, multilingual]
    result["rich-template-edited.pptx"] = [["Revised title", "Revised first paragraph", "Revised second paragraph"], regional, multilingual]
    result["host-roundtrip.pptx"] = [[first[0]] + host + first[2:], regional, multilingual]
    result["host-roundtrip.docx"] = [document[:2] + host + document[5:] + ["Service operations", "Internal review"], next_page + ["Service operations", "Internal review"]]
    result["dense-review.pptx"] = [["Regional delivery review: ownership, remaining risks, customer commitments and next actions"] + [f"Review item {index:02}: confirm the regional delivery date, assigned owner, unresolved risks and next action." for index in range(1, 17)]]
    return result


def read_pages(pdf):
    subprocess.run(["pdftotext", "-bbox-layout", str(pdf), str(pdf.with_suffix(".html"))], check=True, timeout=30)
    subprocess.run(["pdftotext", "-layout", str(pdf), str(pdf.with_suffix(".txt"))], check=True, timeout=30)
    fonts = subprocess.run(["pdffonts", str(pdf)], check=True, capture_output=True, text=True, timeout=30)
    pdf.with_suffix(".fonts.txt").write_text(fonts.stdout)
    boxes = ET.parse(pdf.with_suffix(".html")).findall(".//" + XHTML + "page")
    text = pdf.with_suffix(".txt").read_text().split("\f")
    if text[-1].strip() or len(text) != len(boxes) + 1:
        raise AssertionError(f"{pdf.name}: inconsistent PDF page extraction")
    return [{"width": float(page.attrib["width"]), "height": float(page.attrib["height"]), "text": normalize(text[index]), "words": [{"text": word.text or "", **{key: float(word.attrib[key]) for key in ("xMin", "yMin", "xMax", "yMax")}} for word in page.iter(XHTML + "word")]} for index, page in enumerate(boxes)]


def check_table(page, name):
    words = page["words"]
    def find(text, below=0):
        matches = [word for word in words if word["text"] == text and word["yMin"] > below]
        if len(matches) != 1:
            raise AssertionError(f"{name}: table text {text!r} missing or ambiguous")
        return matches[0]
    region = find("Region")
    top = region["yMin"]
    headers = [region, find("Availability", top - 1), find("Status", top - 1)]
    if max(item["yMin"] for item in headers) - min(item["yMin"] for item in headers) > 2:
        raise AssertionError(f"{name}: table headers no longer share a row")
    if not headers[0]["xMax"] < headers[1]["xMin"] < headers[1]["xMax"] < headers[2]["xMin"]:
        raise AssertionError(f"{name}: table columns overlap or changed order")
    rows = [[find(value, top + 2) for value in values] for values in (("APAC", "99.7%", "Open"), ("EMEA", "99.9%", "Complete"))]
    for row in rows:
        if max(item["yMin"] for item in row) - min(item["yMin"] for item in row) > 2:
            raise AssertionError(f"{name}: table cells no longer share a row")
        if any(abs(cell["xMin"] - header["xMin"]) > 3 for header, cell in zip(headers, row)):
            raise AssertionError(f"{name}: table cells no longer align with their headers")
    due = find("Due", top + 2)
    if not rows[0][0]["yMin"] < due["yMin"] < rows[1][0]["yMin"] or abs(due["xMin"] - headers[2]["xMin"]) > 3:
        raise AssertionError(f"{name}: multiline table cell moved outside its row/column")


def verify(documents, expected):
    if set(documents) != set(expected):
        raise AssertionError("Every fixture must have a rendered document and layout expectations")
    for name, pages in documents.items():
        if len(pages) != len(expected[name]):
            raise AssertionError(f"{name}: expected {len(expected[name])} pages, got {len(pages)}")
        for index, (page, spans) in enumerate(zip(pages, expected[name])):
            where = f"{name}, page {index + 1}"
            for span in spans:
                if normalize(span) not in page["text"]:
                    raise AssertionError(f"{where}: missing text {span!r}")
            if not page["words"] or not all(math.isfinite(page[key]) and page[key] > 0 for key in ("width", "height")):
                raise AssertionError(f"{where}: invalid or empty page")
            for word in page["words"]:
                x1, y1, x2, y2 = (word[key] for key in ("xMin", "yMin", "xMax", "yMax"))
                if not all(math.isfinite(value) for value in (x1, y1, x2, y2)) or not (0 <= x1 < x2 <= page["width"] and 0 <= y1 < y2 <= page["height"]):
                    raise AssertionError(f"{where}: text outside page: {word['text']!r}")
                # These synthetic decks use known title/body rectangles in points.
                # Six points allow font ascender/descender metrics, including Arabic.
                if name.endswith(".pptx") and not (54 <= x1 < x2 <= 906 and (30 <= y1 < y2 <= 114 or 120 <= y1 < y2 <= 492)):
                    raise AssertionError(f"{where}: text outside title/body bounds: {word['text']!r}")
        if name.endswith(".docx"):
            check_table(pages[0], name)
            if "Customer review" in pages[0]["text"] or "APAC" in pages[1]["text"]:
                raise AssertionError(f"{name}: content crossed the explicit page break")


def negative_controls(documents, expected):
    mutations = {
        "missing dense paragraph": lambda data: data["dense-review.pptx"][0].update(text=data["dense-review.pptx"][0]["text"].replace("Review item 16:", "Removed:")),
        "reordered slides": lambda data: data["professional-edited.pptx"].reverse(),
        "text outside page": lambda data: data["professional-new.pptx"][0]["words"][0].update(xMax=10000),
        "misaligned table cell": lambda data: next(word for word in data["professional-new.docx"][0]["words"] if word["text"] == "EMEA").update(xMin=100, xMax=140),
    }
    for name, mutate in mutations.items():
        altered = copy.deepcopy(documents)
        mutate(altered)
        try:
            verify(altered, expected)
        except AssertionError:
            continue
        raise AssertionError(f"Verifier accepted negative control: {name}")
    return list(mutations)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("fixtures", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--soffice", default="libreoffice")
    args = parser.parse_args()
    engine = shutil.which(args.soffice)
    if not engine or not all(shutil.which(tool) for tool in ("pdftotext", "pdftoppm", "pdffonts")):
        parser.error("Install LibreOffice Writer/Impress and Poppler (pdftotext/pdftoppm/pdffonts).")
    fixtures, output = args.fixtures.resolve(), args.output.resolve()
    expected = expectations()
    if set(expected) != {path.name for path in fixtures.iterdir() if path.suffix in (".docx", ".pptx")}:
        raise AssertionError("Use a complete, dedicated fixture directory, including shared-editor roundtrips; every DOCX/PPTX needs expectations")
    digests = {name: hashlib.sha256((fixtures / name).read_bytes()).hexdigest() for name in expected}
    # Refuse stale output; LO can report success even when a conversion fails.
    output.mkdir(parents=True, exist_ok=False)
    version = subprocess.run([engine, "--version"], check=True, capture_output=True, text=True, timeout=20).stdout.strip()
    documents = {}
    for extension, filter_name in (("docx", "writer_pdf_Export"), ("pptx", "impress_pdf_Export")):
        destination = output / extension
        destination.mkdir()
        inputs = sorted(fixtures.glob("*." + extension))
        with tempfile.TemporaryDirectory(prefix="gator-layout-profile-") as profile:
            result = subprocess.run([engine, "-env:UserInstallation=" + Path(profile).as_uri(), "--headless", "--norestore", "--convert-to", "pdf:" + filter_name, "--outdir", str(destination), *map(str, inputs)], check=True, capture_output=True, text=True, timeout=120)
            print(result.stdout, end="", flush=True)
        for original in inputs:
            pdf = destination / (original.stem + ".pdf")
            documents[original.name] = read_pages(pdf)
            subprocess.run(["pdftoppm", "-scale-to", "1400", "-png", str(pdf), str(destination / original.stem)], check=True, timeout=60)
    verify(documents, expected)
    controls = negative_controls(documents, expected)
    for name, digest in digests.items():
        if hashlib.sha256((fixtures / name).read_bytes()).hexdigest() != digest:
            raise AssertionError(f"Original file changed: {name}")
    report = {"engine": version, "files": {name: {"pages": len(pages), "checked_spans": sum(map(len, expected[name])), "words": sum(len(page['words']) for page in pages)} for name, pages in documents.items()}, "input_sha256": digests, "negative_controls_rejected": controls, "scope": "Synthetic page/slide order, expected text, page and slide-box bounds, table alignment. Images require human review; native Office, general typography and arbitrary templates remain unverified."}
    (output / "layout-report.json").write_text(json.dumps(report, indent=2) + "\n")
    print(f"PASS: {len(documents)} files, {sum(len(pages) for pages in documents.values())} pages; text, bounds, tables and {len(controls)} negative controls; originals unchanged. Engine: {version}")


if __name__ == "__main__":
    main()
