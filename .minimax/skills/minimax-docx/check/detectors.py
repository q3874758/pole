"""Validation detectors for identifying document quality issues."""

from functools import cached_property
from pathlib import Path
from typing import Protocol
import xml.etree.ElementTree as ET

from loguru import logger

from .report import ValidationReport

# XML namespaces
WML = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
REL = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
WP = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"


class Detector(Protocol):
    """Interface contract for all validation detectors."""
    name: str

    def scan(self, ctx: "ScanContext") -> None:
        """Execute detection logic and record findings to context report."""
        ...


class ScanContext:
    """Provides unified access to document parts during validation.

    Lazily loads and caches document components to avoid redundant parsing
    when multiple detectors examine the same content.
    """

    def __init__(self, pkg_dir: Path, report: ValidationReport):
        self._pkg_dir = pkg_dir
        self.report = report

    @cached_property
    def document_root(self) -> ET.Element:
        """Parse and return the main document.xml root element."""
        doc_path = self._pkg_dir / "word" / "document.xml"
        if not doc_path.exists():
            raise FileNotFoundError(f"Missing document.xml in {self._pkg_dir}")
        return ET.parse(doc_path).getroot()

    @cached_property
    def parent_map(self) -> dict[ET.Element, ET.Element]:
        """Build child-to-parent mapping for element traversal."""
        return {child: parent for parent in self.document_root.iter() for child in parent}

    @cached_property
    def relationships(self) -> dict[str, str]:
        """Build mapping from relationship ID to target path."""
        rels_path = self._pkg_dir / "word" / "_rels" / "document.xml.rels"
        if not rels_path.exists():
            return {}

        result = {}
        tree = ET.parse(rels_path)
        for rel in tree.findall(".//{http://schemas.openxmlformats.org/package/2006/relationships}Relationship"):
            rid = rel.get("Id", "")
            target = rel.get("Target", "")
            if rid and target:
                result[rid] = target
        return result

    @property
    def word_dir(self) -> Path:
        """Path to the word/ subdirectory."""
        return self._pkg_dir / "word"


class GridConsistencyDetector:
    """Verifies table grid definitions align with actual cell widths.

    Tables in OOXML define column widths in tblGrid, and each cell can
    specify its own width. Mismatches cause rendering unpredictability.
    """
    name = "grid-consistency"

    def scan(self, ctx: ScanContext) -> None:
        tables = ctx.document_root.findall(f".//{{{WML}}}tbl")

        for idx, tbl in enumerate(tables, 1):
            grid = tbl.find(f"{{{WML}}}tblGrid")
            if grid is None:
                continue

            grid_cols = grid.findall(f"{{{WML}}}gridCol")
            defined_widths = []
            for col in grid_cols:
                w = col.get(f"{{{WML}}}w")
                if w and w.isdigit():
                    defined_widths.append(int(w))

            if not defined_widths:
                continue

            for row_idx, tr in enumerate(tbl.findall(f"{{{WML}}}tr"), 1):
                cells = tr.findall(f"{{{WML}}}tc")
                col_cursor = 0

                for tc in cells:
                    tc_pr = tc.find(f"{{{WML}}}tcPr")
                    if tc_pr is None:
                        col_cursor += 1
                        continue

                    span_elem = tc_pr.find(f"{{{WML}}}gridSpan")
                    span = 1
                    if span_elem is not None:
                        val = span_elem.get(f"{{{WML}}}val")
                        if val and val.isdigit():
                            span = int(val)

                    tc_w = tc_pr.find(f"{{{WML}}}tcW")
                    if tc_w is not None:
                        cell_width = tc_w.get(f"{{{WML}}}w")
                        if cell_width and cell_width.isdigit():
                            expected = sum(defined_widths[col_cursor:col_cursor + span])
                            actual = int(cell_width)
                            # Use 8% tolerance to allow for rounding in grid calculations
                            if expected > 0 and abs(actual - expected) / expected > 0.08:
                                ctx.report.warning(
                                    f"table[{idx}]/row[{row_idx}]",
                                    f"Cell width {actual} deviates from grid sum {expected}"
                                )

                    col_cursor += span


class AspectRatioDetector:
    """Checks that embedded images preserve their original proportions.

    Distorted images indicate cx/cy values were modified without
    maintaining the source aspect ratio.
    """
    name = "aspect-ratio"

    def scan(self, ctx: ScanContext) -> None:
        try:
            from PIL import Image
        except ImportError:
            return

        drawings = ctx.document_root.findall(".//{http://schemas.openxmlformats.org/drawingml/2006/main}blip")

        for blip in drawings:
            embed = blip.get("{http://schemas.openxmlformats.org/officeDocument/2006/relationships}embed")
            if not embed or embed not in ctx.relationships:
                continue

            target = ctx.relationships[embed]
            img_path = ctx.word_dir / target

            if not img_path.exists():
                continue

            try:
                with Image.open(img_path) as img:
                    src_w, src_h = img.size
                    if src_h == 0:
                        continue
                    src_ratio = src_w / src_h
            except Exception:
                logger.exception(f"Failed to open image: {img_path}")
                continue

            extent = ctx.parent_map.get(blip)
            while extent is not None and not extent.tag.endswith("}extent"):
                extent = ctx.parent_map.get(extent)

            if extent is None:
                continue

            cx = extent.get("cx")
            cy = extent.get("cy")
            if not cx or not cy:
                continue

            try:
                doc_ratio = int(cx) / int(cy)
            except (ValueError, ZeroDivisionError):
                continue

            # Allow 3% deviation for minor rounding differences
            if abs(src_ratio - doc_ratio) / src_ratio > 0.03:
                ctx.report.warning(
                    f"image/{embed}",
                    f"Aspect ratio changed from {src_ratio:.2f} to {doc_ratio:.2f}"
                )


class AnnotationLinkDetector:
    """Validates comment references have corresponding definitions.

    Comments in OOXML span from commentRangeStart to commentRangeEnd,
    referencing entries in comments.xml. Orphaned references cause errors.
    """
    name = "annotation-links"

    def scan(self, ctx: ScanContext) -> None:
        comments_file = ctx.word_dir / "comments.xml"

        range_starts = ctx.document_root.findall(f".//{{{WML}}}commentRangeStart")
        referenced_ids = {rs.get(f"{{{WML}}}id") for rs in range_starts if rs.get(f"{{{WML}}}id")}

        if not referenced_ids:
            return

        if not comments_file.exists():
            for rid in referenced_ids:
                ctx.report.blocker(
                    f"comment/{rid}",
                    "Comment reference exists but comments.xml is missing"
                )
            return

        comments_tree = ET.parse(comments_file)
        defined_ids = set()
        for comment in comments_tree.findall(f".//{{{WML}}}comment"):
            cid = comment.get(f"{{{WML}}}id")
            if cid:
                defined_ids.add(cid)

        orphans = referenced_ids - defined_ids
        for oid in orphans:
            ctx.report.blocker(
                f"comment/{oid}",
                "Reference points to undefined comment entry"
            )


class BookmarkIntegrityDetector:
    """Validates bookmark start/end pairs are properly matched.

    Bookmarks require both bookmarkStart and bookmarkEnd with matching IDs.
    Unmatched bookmarks cause cross-reference failures.
    """
    name = "bookmark-integrity"

    def scan(self, ctx: ScanContext) -> None:
        starts = ctx.document_root.findall(f".//{{{WML}}}bookmarkStart")
        ends = ctx.document_root.findall(f".//{{{WML}}}bookmarkEnd")

        start_ids = {s.get(f"{{{WML}}}id") for s in starts if s.get(f"{{{WML}}}id")}
        end_ids = {e.get(f"{{{WML}}}id") for e in ends if e.get(f"{{{WML}}}id")}

        # Check for orphaned starts (no matching end)
        orphan_starts = start_ids - end_ids
        for oid in orphan_starts:
            ctx.report.warning(
                f"bookmark/{oid}",
                "bookmarkStart has no matching bookmarkEnd"
            )

        # Check for orphaned ends (no matching start)
        orphan_ends = end_ids - start_ids
        for oid in orphan_ends:
            ctx.report.warning(
                f"bookmark/{oid}",
                "bookmarkEnd has no matching bookmarkStart"
            )


class DrawingIdUniquenessDetector:
    """Ensures drawing element IDs are unique across the document.

    Duplicate docPr id values cause rendering issues in some viewers
    and may result in images not displaying correctly.
    """
    name = "drawing-id-uniqueness"

    def scan(self, ctx: ScanContext) -> None:
        doc_prs = ctx.document_root.findall(f".//{{{WP}}}docPr")

        seen_ids: dict[str, int] = {}
        for doc_pr in doc_prs:
            id_val = doc_pr.get("id")
            if not id_val:
                continue

            if id_val in seen_ids:
                seen_ids[id_val] += 1
            else:
                seen_ids[id_val] = 1

        for id_val, count in seen_ids.items():
            if count > 1:
                ctx.report.warning(
                    f"drawing/docPr[@id={id_val}]",
                    f"Duplicate drawing ID found {count} times"
                )


class HyperlinkValidityDetector:
    """Checks that hyperlinks have valid relationship targets.

    Hyperlinks referencing missing relationships will not work
    when clicked in Word.
    """
    name = "hyperlink-validity"

    def scan(self, ctx: ScanContext) -> None:
        hyperlinks = ctx.document_root.findall(f".//{{{WML}}}hyperlink")

        for hl in hyperlinks:
            rid = hl.get(f"{{{REL}}}id")
            if rid and rid not in ctx.relationships:
                anchor = hl.get(f"{{{WML}}}anchor", "")
                if not anchor:  # Only flag if no internal anchor either
                    ctx.report.warning(
                        f"hyperlink/{rid}",
                        "Hyperlink references missing relationship"
                    )
