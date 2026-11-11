import importlib.util
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("check_forbidden_dom_injection.py")
SPEC = importlib.util.spec_from_file_location(
    "check_forbidden_dom_injection", SCRIPT_PATH
)
CHECKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKER)


class ForbiddenDomInjectionTest(unittest.TestCase):
    def test_rejects_react_raw_html_prop(self):
        prop = "dangerouslySet" + "InnerHTML"

        self.assertEqual(
            {("src/Example.tsx", 1): {"React dangerouslySetInnerHTML prop"}},
            CHECKER._find_matches("src/Example.tsx", f"<div {prop}={{value}} />"),
        )

    def test_rejects_dom_html_sinks(self):
        inner_property = "inner" + "HTML"
        insert_method = "insertAdjacent" + "HTML"
        content = (
            f"node.{inner_property} = value;"
            f"\nnode.{insert_method}('beforeend', value);"
        )

        self.assertEqual(
            {
                ("src/Example.tsx", 1): {"DOM innerHTML or outerHTML property"},
                ("src/Example.tsx", 2): {"DOM insertAdjacentHTML method"},
            },
            CHECKER._find_matches("src/Example.tsx", content),
        )

    def test_allows_normal_react_elements(self):
        self.assertEqual(
            {},
            CHECKER._find_matches("src/Example.tsx", "<Text content={value} />"),
        )

    def test_rejects_other_dom_injection_sinks(self):
        content = (
            "document.write(value);\n"
            "range.createContextualFragment(value);\n"
            "container.html(value);\n"
            "node.outerHTML = value;"
        )

        self.assertEqual(
            {
                ("src/Example.tsx", 1): {"document.write method"},
                ("src/Example.tsx", 2): {
                    "DOM createContextualFragment method"
                },
                ("src/Example.tsx", 3): {"jQuery-style html method"},
                ("src/Example.tsx", 4): {
                    "DOM innerHTML or outerHTML property"
                },
            },
            CHECKER._find_matches("src/Example.tsx", content),
        )

    def test_scans_browser_source_and_mockup_files(self):
        self.assertTrue(CHECKER._scannable("components/app/page.tsx"))
        self.assertTrue(CHECKER._scannable("specs/web-console/mockups/Page.tsx"))
        self.assertFalse(CHECKER._scannable("docs/security.md"))


if __name__ == "__main__":
    unittest.main()
