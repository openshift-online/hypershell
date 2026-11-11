/** One syntax-highlighted run of source text. */
export interface SourceToken {
  text: string;
  lightColor?: string;
  darkColor?: string;
  italic?: boolean;
  bold?: boolean;
  underline?: boolean;
  strikethrough?: boolean;
}

/** Tokens on one source line. */
export interface SourceLine {
  tokens: SourceToken[];
}

/** Tokenized source, independent of React or Shiki’s HTML renderer. */
export interface SourceHighlight {
  lines: SourceLine[];
  foreground?: string;
  darkForeground?: string;
  background?: string;
  darkBackground?: string;
}
