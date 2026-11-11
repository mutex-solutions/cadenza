/**
 * Markdown the server already rendered and sanitized with an allow-list
 * (summary_html, text_html). Links open in a new tab.
 */
export function Html({ html, className }: { html: string; className?: string }) {
  return <div className={className} dangerouslySetInnerHTML={{ __html: withSafeLinks(html) }} />;
}

export function withSafeLinks(html: string): string {
  return html.replace(/<a\s/gi, '<a target="_blank" rel="noopener noreferrer" ');
}
