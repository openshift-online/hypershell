import type { MetaFunction } from "react-router";

import { englishMessages, type EnglishMessageId } from "../i18n/catalog";

export function createPageMeta(
  titleId: EnglishMessageId,
  descriptionId: EnglishMessageId,
  productId: EnglishMessageId = "app.productName",
): MetaFunction {
  return () => [
    {
      title: `${englishMessages[productId]} - ${englishMessages[titleId]}`,
    },
    { name: "description", content: englishMessages[descriptionId] },
  ];
}
