import type { MetaFunction } from "react-router";

import { englishMessages, type EnglishMessageId } from "../i18n/catalog";

export function createPageMeta(
  titleId: EnglishMessageId,
  descriptionId: EnglishMessageId,
): MetaFunction {
  return () => [
    {
      title: `${englishMessages["app.productName"]} — ${englishMessages[titleId]}`,
    },
    { name: "description", content: englishMessages[descriptionId] },
  ];
}
