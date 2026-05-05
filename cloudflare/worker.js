// vps-health installer redirect.
//
// Routes:
//   GET ottomind.ai/vh        → installer script (text/plain so curl|bash works)
//   GET ottomind.ai/vh/       → same
//   GET ottomind.ai/vh/v0.1.0 → installer pinned to that version (sets VPS_HEALTH_VERSION)
//
// Route binding (in dashboard or wrangler.toml):
//   pattern: "ottomind.ai/vh*"  zone: "ottomind.ai"

const REPO = "19011022/vps-health";
const SCRIPT_URL =
  `https://raw.githubusercontent.com/${REPO}/main/install.sh`;

export default {
  async fetch(req) {
    const url = new URL(req.url);
    const path = url.pathname.replace(/\/+$/, ""); // strip trailing slash

    if (path !== "/vh" && !path.startsWith("/vh/")) {
      return new Response("Not Found", { status: 404 });
    }

    // Optional version pin from path: /vh/v0.1.0 or /vh/0.1.0
    let pinned = "";
    if (path.startsWith("/vh/")) {
      pinned = path.slice("/vh/".length).replace(/^v/, "");
    }

    // Fetch upstream installer.
    const upstream = await fetch(SCRIPT_URL, {
      cf: { cacheTtl: 300, cacheEverything: true },
    });
    if (!upstream.ok) {
      return new Response(
        `# Failed to fetch installer (HTTP ${upstream.status})\n` +
          `# Try directly: curl -fsSL ${SCRIPT_URL} | bash\n`,
        { status: 502, headers: { "content-type": "text/plain; charset=utf-8" } },
      );
    }

    let body = await upstream.text();

    // If a version was pinned in the URL, prepend an export so the user
    // doesn't have to set VPS_HEALTH_VERSION themselves.
    if (pinned && /^[0-9][0-9A-Za-z.\-]*$/.test(pinned)) {
      body = body.replace(
        /^#!\/usr\/bin\/env bash\n/,
        `#!/usr/bin/env bash\nexport VPS_HEALTH_VERSION="${pinned}"\n`,
      );
    }

    return new Response(body, {
      status: 200,
      headers: {
        "content-type": "text/plain; charset=utf-8",
        "cache-control": "public, max-age=300",
        "x-source": SCRIPT_URL,
      },
    });
  },
};
