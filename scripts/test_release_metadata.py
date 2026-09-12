#!/usr/bin/env python3
"""Exercise tag validation independently of the current release number."""
from pathlib import Path
import tempfile
import unittest
from release_metadata import validate


class ReleaseMetadataTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "deployments/helm/aegisflow").mkdir(parents=True)
        (self.root / "docs/releases").mkdir(parents=True)
        self.chart = self.root / "deployments/helm/aegisflow/Chart.yaml"
        self.chart.write_text('version: 1.2.3\nappVersion: "1.2.3"\n')
        (self.root / "CHANGELOG.md").write_text("## [1.2.3] - 2026-09-12\n")
        self.notes = self.root / "docs/releases/v1.2.3.md"
        self.notes.write_text("# AegisFlow v1.2.3: release\n")

    def test_future_version(self):
        self.assertEqual(validate(self.root, "v1.2.3")["sbom"], "aegisflow-v1.2.3.spdx.json")

    def test_unsafe_and_unsupported_tags(self):
        for tag in ("1.2.3", "v01.2.3", "v1.2", "v1.2.3-rc.1", "v1.2.3\n", "v1.2.3;id", "../../file"):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                validate(self.root, tag)

    def test_chart_disagreement(self):
        for chart in ('version: 1.2.2\nappVersion: "1.2.3"\n', 'version: 1.2.3\nappVersion: "1.2.2"\n', 'version: 1.2.3\nversion: 1.2.3\nappVersion: "1.2.3"\n', 'version: 1.2.3\nappVersion: "1.2.3\n'):
            self.chart.write_text(chart)
            with self.assertRaises(ValueError):
                validate(self.root, "v1.2.3")

    def test_missing_or_wrong_notes(self):
        self.notes.write_text("# AegisFlow v1.2.2: release\n")
        with self.assertRaises(ValueError):
            validate(self.root, "v1.2.3")
        self.notes.unlink()
        with self.assertRaises(OSError):
            validate(self.root, "v1.2.3")

    def test_changelog_date_and_duplicates(self):
        for text in ("## [1.2.3] - 2026-02-30\n", "## [1.2.2] - 2026-09-12\n", "## [1.2.3] - 2026-09-12\n" * 2):
            (self.root / "CHANGELOG.md").write_text(text)
            with self.assertRaises(ValueError):
                validate(self.root, "v1.2.3")


if __name__ == "__main__":
    unittest.main()
