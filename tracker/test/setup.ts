import { test as base } from '@playwright/test';

/**
 * Test server that hosts HTML pages and captures tracking requests
 */
export interface TestContext {
  capturedRequests: any[];
  testPageUrl: string;
  trackerUrl: string;
}

export const test = base.extend<TestContext>({
  capturedRequests: [],
  testPageUrl: 'http://localhost:5173',
  trackerUrl: 'http://localhost:5173/k.js',
});

export { expect } from '@playwright/test';

/**
 * Create a test HTML page that loads the tracker
 */
export function createTestHtmlPage(
  loadingMode: 'defer' | 'async' | 'inline' = 'defer',
  dataAttributes: Record<string, string> = {}
): string {
  const attrs = Object.entries(dataAttributes)
    .map(([key, value]) => `data-${key}="${value}"`)
    .join(' ');

  const scriptTag = (() => {
    const src = 'http://localhost:5173/k.js';
    // Only add default website-id if not provided in attrs
    const defaultWebsiteId = attrs.includes('data-website-id') ? '' : 'data-website-id="test-website-123"';
    const baseAttrs = `src="${src}" ${defaultWebsiteId} ${attrs}`.replace(/\s+/g, ' ').trim();

    switch (loadingMode) {
      case 'defer':
        return `<script defer ${baseAttrs}></script>`;
      case 'async':
        return `<script async ${baseAttrs}></script>`;
      case 'inline':
        return `<script ${baseAttrs}></script>`;
    }
  })();

  return `
    <!DOCTYPE html>
    <html>
    <head>
      <title>Test Page</title>
    </head>
    <body>
      <h1>Test Page for Kaunta Tracker</h1>
      <p>This page loads the Kaunta tracking script.</p>
      ${scriptTag}
      <script>
        window.__capturedRequests = [];
        const originalFetch = window.fetch;
        window.fetch = function(...args) {
          window.__capturedRequests.push({
            url: args[0],
            init: args[1],
            timestamp: Date.now()
          });
          return originalFetch.apply(this, args);
        };
      </script>
    </body>
    </html>
  `;
}
