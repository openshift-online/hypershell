import { defineMessages } from "react-intl";

export const messages = defineMessages({
  catalogSummary: {
    id: "specViewer.catalog.summary",
    defaultMessage: "{files}, {specs}, {scenarios}",
    description: "Counts of parsed spec files, specs, and scenarios.",
  },
  citationsMore: {
    id: "specViewer.conformance.citationsMore",
    defaultMessage: "{remaining} more",
    description:
      "Collapsed label group text; {remaining} is filled in by PatternFly.",
  },
  clearPins: {
    id: "specViewer.pins.clear",
    defaultMessage: "Clear",
    description: "Button that removes every pinned item.",
  },
  collapseAll: {
    id: "specViewer.search.collapseAll",
    defaultMessage: "Collapse all",
    description: "Link that collapses every search result to its file.",
  },
  columnConformance: {
    id: "specViewer.conformance.column.conformance",
    defaultMessage: "Conformance",
    description:
      "Column header and metric for the share of conforming requirements.",
  },
  columnMissingId: {
    id: "specViewer.conformance.column.missingId",
    defaultMessage: "Missing ID",
    description: "Column tooltip and metric for requirements without an ID.",
  },
  columnMissingIdShort: {
    id: "specViewer.conformance.column.missingIdShort",
    defaultMessage: "No ID",
    description: "Short column header for requirements without an ID.",
  },
  columnName: {
    id: "specViewer.conformance.column.name",
    defaultMessage: "Name",
    description: "Column header for folder, file, and requirement names.",
  },
  columnRequirements: {
    id: "specViewer.conformance.column.requirements",
    defaultMessage: "Requirements",
    description: "Column header and metric for the number of requirements.",
  },
  columnRequirementsShort: {
    id: "specViewer.conformance.column.requirementsShort",
    defaultMessage: "Reqs",
    description: "Short column header for the number of requirements.",
  },
  columnScenarioMissingId: {
    id: "specViewer.conformance.column.scenarioMissingId",
    defaultMessage: "Scenarios missing ID",
    description: "Column tooltip and metric for scenarios without an ID.",
  },
  columnScenarioMissingIdShort: {
    id: "specViewer.conformance.column.scenarioMissingIdShort",
    defaultMessage: "No S-ID",
    description: "Short column header for scenarios without an ID.",
  },
  columnScenarios: {
    id: "specViewer.conformance.column.scenarios",
    defaultMessage: "Scenarios",
    description: "Column header for the number of scenarios.",
  },
  columnScenariosShort: {
    id: "specViewer.conformance.column.scenariosShort",
    defaultMessage: "Scen.",
    description: "Short column header for the number of scenarios.",
  },
  columnTested: {
    id: "specViewer.conformance.column.tested",
    defaultMessage: "Scenarios cited by a test (Verifies: marker)",
    description: "Column tooltip and metric for scenarios a test marker cites.",
  },
  columnTestedShort: {
    id: "specViewer.conformance.column.testedShort",
    defaultMessage: "Tested",
    description: "Short column header for scenarios cited by a test marker.",
  },
  columnWellFormed: {
    id: "specViewer.conformance.column.wellFormed",
    defaultMessage: "Well-formed scenarios",
    description:
      "Column tooltip and metric for scenarios with a WHEN step followed by a THEN step.",
  },
  columnWellFormedShort: {
    id: "specViewer.conformance.column.wellFormedShort",
    defaultMessage: "OK scen.",
    description: "Short column header for well-formed scenarios.",
  },
  conformanceLabel: {
    id: "specViewer.conformance.label",
    defaultMessage: "Spec conformance",
    description: "Accessible label of the conformance table.",
  },
  conformanceOf: {
    id: "specViewer.conformance.of",
    defaultMessage: "Conformance of {name}",
    description: "Accessible label of a conformance bar.",
  },
  countFiles: {
    id: "specViewer.count.files",
    defaultMessage: "{count, plural, one {# file} other {# files}}",
    description: "Number of spec files.",
  },
  countMatches: {
    id: "specViewer.count.matches",
    defaultMessage: "{count, plural, one {# match} other {# matches}}",
    description: "Number of search results.",
  },
  countScenarios: {
    id: "specViewer.count.scenarios",
    defaultMessage: "{count, plural, one {# scenario} other {# scenarios}}",
    description: "Number of scenarios.",
  },
  countSpecs: {
    id: "specViewer.count.specs",
    defaultMessage: "{count, plural, one {# spec} other {# specs}}",
    description: "Number of specs (requirements).",
  },
  documentEmptyBody: {
    id: "specViewer.document.empty.body",
    defaultMessage:
      "Select a search result or a pinned item to open its spec file.",
    description: "Body of the empty document panel.",
  },
  documentEmptyTitle: {
    id: "specViewer.document.empty.title",
    defaultMessage: "No spec selected",
    description: "Title of the empty document panel.",
  },
  documentLabel: {
    id: "specViewer.document.label",
    defaultMessage: "Spec document",
    description: "Accessible label of the spec document panel.",
  },
  expandAll: {
    id: "specViewer.search.expandAll",
    defaultMessage: "Expand all",
    description: "Link that expands every search result.",
  },
  filesWithoutRequirements: {
    id: "specViewer.conformance.filesWithoutRequirements",
    defaultMessage:
      "{count, plural, one {# file has} other {# files have}} no requirements: {files}.",
    description: "Note listing spec files that contain no requirements.",
  },
  issueMalformedScenarios: {
    id: "specViewer.conformance.issue.malformedScenarios",
    defaultMessage: "malformed scenarios",
    description:
      "Issue: some scenarios of a requirement lack WHEN/THEN structure.",
  },
  issueMissingId: {
    id: "specViewer.conformance.issue.missingId",
    defaultMessage: "no ID",
    description: "Issue: a requirement heading has no requirement ID.",
  },
  issueMissingThen: {
    id: "specViewer.conformance.issue.missingThen",
    defaultMessage: "missing THEN",
    description: "Issue: a scenario has no THEN step.",
  },
  issueMissingWhen: {
    id: "specViewer.conformance.issue.missingWhen",
    defaultMessage: "missing WHEN",
    description: "Issue: a scenario has no WHEN step.",
  },
  issueNone: {
    id: "specViewer.conformance.issue.none",
    defaultMessage: "ok",
    description: "Label of a requirement or scenario that passes every check.",
  },
  issueNoNormative: {
    id: "specViewer.conformance.issue.noNormative",
    defaultMessage: "no SHALL/MUST",
    description: "Issue: a requirement uses no normative keyword.",
  },
  issueNoScenarios: {
    id: "specViewer.conformance.issue.noScenarios",
    defaultMessage: "no scenarios",
    description: "Issue: a requirement has no scenarios.",
  },
  issueScenariosMissingId: {
    id: "specViewer.conformance.issue.scenariosMissingId",
    defaultMessage: "scenario without ID",
    description: "Issue: some scenarios of a requirement have no scenario ID.",
  },
  issueThenBeforeWhen: {
    id: "specViewer.conformance.issue.thenBeforeWhen",
    defaultMessage: "THEN before WHEN",
    description: "Issue: a scenario states THEN before WHEN.",
  },
  issueUntested: {
    id: "specViewer.conformance.issue.untested",
    defaultMessage: "no test",
    description: "Issue: no test carries a Verifies marker for this scenario.",
  },
  issueUntestedScenarios: {
    id: "specViewer.conformance.issue.untestedScenarios",
    defaultMessage: "untested scenarios",
    description:
      "Issue: some scenarios of a requirement have no test citing them.",
  },
  loadErrorTitle: {
    id: "specViewer.load.error.title",
    defaultMessage: "Could not load the specs",
    description: "Title shown when the specs folder cannot be read.",
  },
  loading: {
    id: "specViewer.load.loading",
    defaultMessage: "Loading specs",
    description: "Accessible label of the loading spinner.",
  },
  metricTested: {
    id: "specViewer.conformance.metric.tested",
    defaultMessage: "Tested scenarios",
    description: "Metric: scenarios a Verifies test marker cites.",
  },
  metricWithScenarios: {
    id: "specViewer.conformance.metric.withScenarios",
    defaultMessage: "With scenarios",
    description: "Metric: requirements that have at least one scenario.",
  },
  noResultsBody: {
    id: "specViewer.search.noResults.body",
    defaultMessage: "Try fewer or shorter words.",
    description: "Hint shown when a search has no results.",
  },
  noResultsTitle: {
    id: "specViewer.search.noResults.title",
    defaultMessage: "No matches",
    description: "Title shown when a search has no results.",
  },
  nospecPrefix: {
    id: "specViewer.node.nospecPrefix",
    defaultMessage: "[nospec] {title}",
    description:
      "Title of a markdown heading that is neither a spec nor a scenario.",
  },
  pin: {
    id: "specViewer.node.pin",
    defaultMessage: "Pin {title}",
    description: "Accessible label of the button that pins one item.",
  },
  pinnedEmpty: {
    id: "specViewer.pins.empty",
    defaultMessage: "Pin search results to keep them here.",
    description: "Hint shown when nothing is pinned.",
  },
  pinnedLabel: {
    id: "specViewer.pins.label",
    defaultMessage: "Pinned specs",
    description: "Accessible label of the pinned specs tree.",
  },
  pinnedTitle: {
    id: "specViewer.pins.title",
    defaultMessage: "Pinned",
    description: "Heading of the pinned specs side panel.",
  },
  pinResults: {
    id: "specViewer.search.pinResults",
    defaultMessage: "Pin results",
    description: "Button that pins every current search result.",
  },
  productName: {
    id: "specViewer.productName",
    defaultMessage: "HyperShell specs",
    description: "Product name shown in the masthead.",
  },
  refDefault: {
    id: "specViewer.ref.default",
    defaultMessage: "Default branch",
    description: "Marks the default branch in the branch picker.",
  },
  referencesErrorTitle: {
    id: "specViewer.references.error",
    defaultMessage: "Could not search the repository",
    description: "Title shown when the repository search fails.",
  },
  referencesLoading: {
    id: "specViewer.references.loading",
    defaultMessage: "Searching the repository",
    description: "Accessible label of the spinner while searching for an ID.",
  },
  referencesNoIdBody: {
    id: "specViewer.references.noId.body",
    defaultMessage:
      "Related files are found by ID, such as ABC-DEF-01 or ABC-DEF-01.01.",
    description: "Body shown when the selected item has no ID to trace.",
  },
  referencesNoIdTitle: {
    id: "specViewer.references.noId.title",
    defaultMessage: "Select a requirement or scenario with an ID",
    description: "Title shown when the selected item has no ID to trace.",
  },
  referencesNoTests: {
    id: "specViewer.references.noTests",
    defaultMessage: "No test references {id} yet",
    description: "Warning shown when no test file mentions the ID.",
  },
  referencesOtherFiles: {
    id: "specViewer.references.otherFiles",
    defaultMessage: "{count, plural, one {# other file} other {# other files}}",
    description: "Heading of the non-test files that mention the ID.",
  },
  referencesTests: {
    id: "specViewer.references.tests",
    defaultMessage: "{count, plural, one {# test file} other {# test files}}",
    description: "Heading of the test files that mention the ID.",
  },
  refLoading: {
    id: "specViewer.ref.loading",
    defaultMessage: "Loading branches...",
    description: "Shown in the branch picker while the branch list loads.",
  },
  refMore: {
    id: "specViewer.ref.more",
    defaultMessage: "Showing {shown} of {total}; type to narrow",
    description: "Shown when the branch list is cut short.",
  },
  refNoResults: {
    id: "specViewer.ref.noResults",
    defaultMessage: "No branch matches",
    description: "Shown when the branch picker filter matches nothing.",
  },
  refPickerLabel: {
    id: "specViewer.ref.pickerLabel",
    defaultMessage: "Branch",
    description: "Accessible label of the branch picker.",
  },
  refWorkingTree: {
    id: "specViewer.ref.workingTree",
    defaultMessage: "Working tree",
    description:
      "Name of the local files, with uncommitted edits, in the branch picker.",
  },
  resizeList: {
    id: "specViewer.resize.list",
    defaultMessage: "Resize the list panel",
    description:
      "Accessible label of the splitter between the list panel and the document.",
  },
  resizePinned: {
    id: "specViewer.resize.pinned",
    defaultMessage: "Resize the pinned panel",
    description: "Accessible label of the splitter next to the pinned panel.",
  },
  resultCount: {
    id: "specViewer.search.resultCount",
    defaultMessage: "{matches} in {files}",
    description: "Number of search results and the files they are in.",
  },
  resultsLabel: {
    id: "specViewer.search.resultsLabel",
    defaultMessage: "Search results",
    description: "Accessible label of the search results tree.",
  },
  scenarioLabel: {
    id: "specViewer.node.scenario",
    defaultMessage: "Scenario",
    description: "Badge marking a scenario in the results and pinned trees.",
  },
  searchHintBody: {
    id: "specViewer.search.hint.body",
    defaultMessage:
      'Every word must appear in the same spec or its scenarios; typos and abbreviations still match. Use "quotes" for exact text and [brackets] for words on one line.',
    description: "Hint shown before the user types a search.",
  },
  searchHintTitle: {
    id: "specViewer.search.hint.title",
    defaultMessage: "Search the specs",
    description: "Title shown before the user types a search.",
  },
  searchLabel: {
    id: "specViewer.search.label",
    defaultMessage: "Search specs",
    description: "Accessible label and placeholder of the search input.",
  },
  sourceErrorTitle: {
    id: "specViewer.source.error",
    defaultMessage: "Could not open the file",
    description: "Title shown when a repository file cannot be read.",
  },
  sourceLabel: {
    id: "specViewer.source.label",
    defaultMessage: "Source file",
    description: "Accessible label of the source file panel.",
  },
  sourceLoading: {
    id: "specViewer.source.loading",
    defaultMessage: "Loading file",
    description: "Accessible label of the spinner while a file loads.",
  },
  testMarkersError: {
    id: "specViewer.conformance.testMarkers.error",
    defaultMessage: "Test markers are unavailable; the tested check is off",
    description: "Warning shown when the traceability report fails.",
  },
  testMarkersLoading: {
    id: "specViewer.conformance.testMarkers.loading",
    defaultMessage: "Loading test markers\u2026",
    description: "Shown while the traceability report runs.",
  },
  testMarkersReload: {
    id: "specViewer.conformance.testMarkers.reload",
    defaultMessage: "Reload test markers",
    description: "Button that re-runs the traceability report.",
  },
  unpin: {
    id: "specViewer.node.unpin",
    defaultMessage: "Unpin {title}",
    description: "Accessible label of the button that unpins one item.",
  },
  unresolvedLink: {
    id: "specViewer.document.unresolvedLink",
    defaultMessage: "Outside the specs folder: {path}",
    description:
      "Tooltip of a link that points to a file the viewer cannot open.",
  },
  verifiedBy: {
    id: "specViewer.conformance.verifiedBy",
    defaultMessage: "Verified by {method}",
    description:
      "Label of a requirement proven without tests, such as by inspection.",
  },
  viewConformance: {
    id: "specViewer.view.conformance",
    defaultMessage: "Conformance",
    description: "View switch option for the conformance report.",
  },
  viewLabel: {
    id: "specViewer.view.label",
    defaultMessage: "View",
    description: "Accessible label of the view switch.",
  },
  viewReferences: {
    id: "specViewer.view.references",
    defaultMessage: "Files referencing {id}",
    description:
      "Accessible label of the view option that lists files mentioning the selected ID.",
  },
  viewSearch: {
    id: "specViewer.view.search",
    defaultMessage: "Search",
    description: "View switch option for searching specs.",
  },
});
