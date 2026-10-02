declare module "*.png" {
  const source: string;
  export default source;
}

declare module "*.svg" {
  const source: string;
  export default source;
}

declare module "*.module.css" {
  const classes: Record<string, string>;
  export default classes;
}
