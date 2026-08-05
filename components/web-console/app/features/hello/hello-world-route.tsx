import type { MetaFunction } from "react-router";

import { englishMessages } from "../../i18n/catalog";
import { HelloWorld } from "./hello-world";

export const meta: MetaFunction = () => [
  {
    title: `${englishMessages["app.productName"]} — ${englishMessages["app.hello.title"]}`,
  },
  { name: "description", content: englishMessages["app.hello.description"] },
];

export default function HelloWorldRoute() {
  return <HelloWorld />;
}
