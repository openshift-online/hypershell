import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

import type { PlatformPolicyEvaluation } from "../../application/platform-policies/platform-policy-types";
import type { CedarSnippetId, CedarSnippetValues } from "./cedar-snippets";
import {
  editorFormValues,
  editorTestValues,
  invalidCedar,
  invalidEvaluation,
  validCedar,
  validEvaluation,
} from "./fixtures/platform-policy-editor-fixtures";
import {
  PlatformPolicyFormPage,
  type PlatformPolicyEditorPanel,
} from "./platform-policy-form-page";

function PlatformPolicyFormProductionPreview({
  cedar,
  evaluation,
  initialPanel,
  initialSnippet,
  initialSnippetValues,
}: {
  cedar: string;
  evaluation: PlatformPolicyEvaluation;
  initialPanel?: PlatformPolicyEditorPanel;
  initialSnippet?: CedarSnippetId;
  initialSnippetValues?: CedarSnippetValues;
}) {
  const [values, setValues] = useState(() => editorFormValues(cedar));
  const [testValues, setTestValues] = useState(editorTestValues);
  return (
    <PlatformPolicyFormPage
      cancelHref="/admin/platform-policies"
      evaluation={evaluation}
      evaluationFailed={false}
      initialPanel={initialPanel}
      initialSnippet={initialSnippet}
      initialSnippetValues={initialSnippetValues}
      isEditing={false}
      isEvaluating={false}
      isSaving={false}
      onCancel={() => undefined}
      onSave={() => undefined}
      onTestValuesChange={setTestValues}
      onValuesChange={setValues}
      schemaVersion="v1"
      testValues={testValues}
      values={values}
    />
  );
}

const meta = {
  title: "Production/Administration/Create platform policy",
  component: PlatformPolicyFormProductionPreview,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PlatformPolicyFormProductionPreview>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { cedar: validCedar, evaluation: validEvaluation },
};

export const ValidationProblems: Story = {
  args: { cedar: invalidCedar, evaluation: invalidEvaluation },
};

export const Snippets: Story = {
  args: {
    cedar: validCedar,
    evaluation: validEvaluation,
    initialPanel: "snippets",
  },
};

export const AddSnippetRule: Story = {
  args: {
    cedar: validCedar,
    evaluation: validEvaluation,
    initialPanel: "snippets",
    initialSnippet: "cpu-request-range",
    initialSnippetValues: { maximum: "4", minimum: "250m" },
  },
};

export const TestRequest: Story = {
  args: {
    cedar: validCedar,
    evaluation: validEvaluation,
    initialPanel: "test",
  },
};
