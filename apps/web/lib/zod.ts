import { z } from "zod";

// Static pages use a strict CSP. Configure this before constructing schemas so
// Zod does not probe or compile dynamic JavaScript in the browser.
z.config({ jitless: true });

export { z };
