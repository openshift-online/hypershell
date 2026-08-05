import englishCatalog from "../../locales/en.json";

export type EnglishMessageId = keyof typeof englishCatalog;

export const englishMessages = Object.fromEntries(
  Object.entries(englishCatalog).map(([id, descriptor]) => [
    id,
    descriptor.defaultMessage,
  ]),
) as Record<EnglishMessageId, string>;
