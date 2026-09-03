#!/usr/bin/env node

import { mkdir, readFile, readdir, rm, writeFile, copyFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { marked } from 'marked';
import { renderMermaidSVG } from 'beautiful-mermaid';

const toolDir = path.dirname(fileURLToPath(import.meta.url));
const sourceDir = path.resolve(toolDir, '../../docs/architecture/source');
const assetDir = path.resolve(toolDir, '../../docs/architecture/assets');
const outputDir = path.resolve(toolDir, '../../dist/architecture');
const checkOnly = process.argv.includes('--check');

const pages = (await readdir(sourceDir))
  .filter((file) => file.endsWith('.md'))
  .sort((left, right) => (left === 'index.md' ? -1 : right === 'index.md' ? 1 : left.localeCompare(right)));

if (pages.length === 0) {
  throw new Error(`No Markdown architecture pages found in ${sourceDir}`);
}

function escapeHtml(value) {
  return String(value).replace(/[&<>'"]/g, (character) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    "'": '&#39;',
    '"': '&quot;',
  })[character]);
}

function pageSlug(file) {
  return file.replace(/\.md$/, '');
}

function parsePage(markdown, file) {
  const titleMatch = markdown.match(/^#\s+(.+?)\s*\n\s*\n/);
  if (!titleMatch) {
    throw new Error(`${file} must begin with a level-one heading`);
  }

  const title = titleMatch[1];
  let body = markdown.slice(titleMatch[0].length).trim();
  const ledeMatch = body.match(/^([\s\S]+?)(?:\n\s*\n|$)/);
  const lede = ledeMatch?.[1]?.trim() ?? '';
  if (ledeMatch) body = body.slice(ledeMatch[0].length).trim();

  const sourceMatch = body.match(/^Authoritative source:\s*(.+)$/m);
  const source = sourceMatch?.[1] ?? '';
  if (sourceMatch) body = body.replace(sourceMatch[0], '').replace(/^\s*\n/, '').trim();

  return { title, lede, source, body, file };
}

function renderMarkdown(page) {
  const renderer = new marked.Renderer();
  renderer.code = ({ text, lang }) => {
    if (lang?.trim().toLowerCase() === 'mermaid') {
      let svg;
      try {
        svg = renderMermaidSVG(text, {
          transparent: false,
          paddingX: 24,
          paddingY: 20,
        });
      } catch (error) {
        throw new Error(`Mermaid render failed in ${page.file}: ${error.message}`, { cause: error });
      }
      return `<section class="diagram-card"><figure aria-label="Architecture diagram"><div class="diagram">${svg}</div><details><summary>View Mermaid source</summary><pre>${escapeHtml(text)}</pre><p><a href="https://agents.craft.do/mermaid/editor" rel="noreferrer">Edit this diagram in Craft Mermaid</a></p></details></figure></section>`;
    }

    const className = lang ? ` class="language-${escapeHtml(lang)}"` : '';
    return `<pre><code${className}>${escapeHtml(text)}</code></pre>`;
  };

  return marked.parse(page.body, { renderer });
}

function navigation(current) {
  return pages.map((file) => {
    const slug = pageSlug(file);
    const page = pageByFile.get(file);
    const label = file === 'index.md' ? 'Overview' : page.title;
    return `<a href="${slug}.html">${escapeHtml(label)}</a>`;
  }).join('');
}

function pageDocument(page) {
  const source = page.source
    ? `<p class="meta">Authoritative source: <code>${escapeHtml(page.source)}</code></p>`
    : '';
  const breadcrumb = page.file === 'index.md'
    ? ''
    : '<p class="breadcrumb"><a href="index.html">Architecture atlas</a> / View</p>';

  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="generator" content="HyperShell architecture site generator">
  <title>${escapeHtml(page.title)} · HyperShell architecture atlas</title>
  <link rel="stylesheet" href="assets/styles.css">
</head>
<body>
<header class="site-header"><div class="header-inner">
  <p class="eyebrow">HyperShell architecture atlas</p>
  <h1>${escapeHtml(page.title)}</h1>
  <p class="lede">${escapeHtml(page.lede)}</p>
  <nav class="page-nav" aria-label="Architecture pages">${navigation(page.file)}</nav>
</div></header>
<main>${breadcrumb}${source}${renderMarkdown(page)}</main>
<footer class="site-footer"><div class="site-footer-inner">Diagrams are rendered as inline SVG with <a href="https://github.com/lukilabs/beautiful-mermaid" rel="noreferrer">beautiful-mermaid</a>, the open-source renderer behind Craft's <a href="https://agents.craft.do/mermaid" rel="noreferrer">Mermaid renderer</a>. Mermaid source is included on every page for review and editing.</div></footer>
</body>
</html>
`;
}

const pageByFile = new Map();
for (const file of pages) {
  const page = parsePage(await readFile(path.join(sourceDir, file), 'utf8'), file);
  pageByFile.set(file, page);
}

const documents = pages.map((file) => ({ file, html: pageDocument(pageByFile.get(file)) }));

if (checkOnly) {
  console.log(`Validated ${documents.length} architecture pages and their Mermaid diagrams.`);
} else {
  await rm(outputDir, { recursive: true, force: true });
  await mkdir(outputDir, { recursive: true });
  await mkdir(path.join(outputDir, 'assets'), { recursive: true });
  await copyFile(path.join(assetDir, 'styles.css'), path.join(outputDir, 'assets/styles.css'));
  for (const { file, html } of documents) {
    await writeFile(path.join(outputDir, `${pageSlug(file)}.html`), html);
  }
  console.log(`Built ${documents.length} architecture pages in ${path.relative(process.cwd(), outputDir)}.`);
}
