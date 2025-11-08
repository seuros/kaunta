/**
 * Simple test server for Playwright tests
 * Serves the tracker script at /k.js
 */

const PORT = 5173;

const server = Bun.serve({
  port: PORT,
  fetch(req) {
    const url = new URL(req.url);

    // Serve tracker script
    if (url.pathname === '/k.js') {
      const trackerPath = `${import.meta.dir}/../kaunta.js`;
      const file = Bun.file(trackerPath);
      return new Response(file, {
        headers: {
          'Content-Type': 'application/javascript',
          'Access-Control-Allow-Origin': '*',
        },
      });
    }

    // Health check endpoint for Playwright
    if (url.pathname === '/' || url.pathname === '/health') {
      return new Response('OK', {
        headers: {
          'Content-Type': 'text/plain',
        },
      });
    }

    // Handle API send endpoint (mock)
    if (url.pathname === '/api/send') {
      if (req.method === 'OPTIONS') {
        return new Response(null, {
          headers: {
            'Access-Control-Allow-Origin': '*',
            'Access-Control-Allow-Methods': 'POST, OPTIONS',
            'Access-Control-Allow-Headers': 'Content-Type',
          },
        });
      }

      // Just accept the tracking request
      return new Response(JSON.stringify({ ok: true }), {
        headers: {
          'Content-Type': 'application/json',
          'Access-Control-Allow-Origin': '*',
        },
      });
    }

    return new Response('Not Found', { status: 404 });
  },
});

console.log(`Test server running at http://localhost:${PORT}`);
