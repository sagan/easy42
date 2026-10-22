import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { Resvg } from '@resvg/resvg-js';
import pngToIco from 'png-to-ico';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const publicDir = path.resolve(__dirname, '../public');

// Master Icon SVG: 512x512
// Features: Modern rounded squircle badge, gradient mesh background, interconnected "42" topology nodes and WireGuard links
const createLogoSvg = (options = { transparent: false, paddingRatio: 0 }) => {
  const { transparent, paddingRatio } = options;
  const size = 512;
  const padding = size * paddingRatio;
  const innerSize = size - padding * 2;
  const scale = innerSize / size;

  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">
  <defs>
    <!-- Background Gradient -->
    <linearGradient id="bgGrad" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#3730A3" />
      <stop offset="50%" stop-color="#4F46E5" />
      <stop offset="100%" stop-color="#0891B2" />
    </linearGradient>

    <!-- Subtle Radial Glow -->
    <radialGradient id="glowGrad" cx="30%" cy="20%" r="65%">
      <stop offset="0%" stop-color="#A5B4FC" stop-opacity="0.35" />
      <stop offset="100%" stop-color="#312E81" stop-opacity="0" />
    </radialGradient>

    <!-- Mesh Link Gradient -->
    <linearGradient id="linkGrad" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#FFFFFF" />
      <stop offset="60%" stop-color="#E0E7FF" />
      <stop offset="100%" stop-color="#67E8F9" />
    </linearGradient>

    <!-- Bridge Tunnel Gradient -->
    <linearGradient id="tunnelGrad" x1="0%" y1="50%" x2="100%" y2="50%">
      <stop offset="0%" stop-color="#38BDF8" stop-opacity="0.9" />
      <stop offset="100%" stop-color="#A78BFA" stop-opacity="0.9" />
    </linearGradient>

    <!-- Drop Shadow for Nodes and Lines -->
    <filter id="meshGlow" x="-20%" y="-20%" width="140%" height="140%">
      <feDropShadow dx="0" dy="4" stdDeviation="6" flood-color="#0F172A" flood-opacity="0.35" />
    </filter>

    <filter id="nodeGlow" x="-50%" y="-50%" width="200%" height="200%">
      <feDropShadow dx="0" dy="2" stdDeviation="4" flood-color="#06B6D4" flood-opacity="0.5" />
    </filter>
  </defs>

  ${
    transparent
      ? ''
      : `
  <!-- Main Badge Container -->
  <rect width="512" height="512" rx="116" fill="url(#bgGrad)" />
  <rect width="512" height="512" rx="116" fill="url(#glowGrad)" />
  <!-- Glassmorphic Rim -->
  <rect x="8" y="8" width="496" height="496" rx="110" fill="none" stroke="rgba(255, 255, 255, 0.22)" stroke-width="5" />
  `
  }

  <g transform="translate(${padding}, ${padding}) scale(${scale})">
    <!-- Overlay Mesh Interconnect Links (Background topology) -->
    <g stroke="url(#tunnelGrad)" stroke-width="12" stroke-linecap="round" stroke-dasharray="14 10" opacity="0.75">
      <!-- Cross link between 4 crossbar and 2 curve -->
      <line x1="250" y1="265" x2="335" y2="305" />
      <!-- Mesh bridge between top nodes -->
      <line x1="165" y1="125" x2="355" y2="125" opacity="0.4" stroke-dasharray="8 8" stroke-width="8" />
      <!-- Base connection between 4 stem and 2 base -->
      <line x1="230" y1="395" x2="290" y2="395" stroke-dasharray="none" stroke-width="14" opacity="0.9" />
    </g>

    <!-- Main WireGuard Topology Paths -->
    <g filter="url(#meshGlow)">
      <!-- Numeral 4 Structure -->
      <!-- Vertical Stem of 4 -->
      <line x1="230" y1="145" x2="230" y2="395" stroke="url(#linkGrad)" stroke-width="28" stroke-linecap="round" />
      <!-- Diagonal and Horizontal Bar of 4 -->
      <polyline points="165,125 100,265 260,265" fill="none" stroke="url(#linkGrad)" stroke-width="28" stroke-linecap="round" stroke-linejoin="round" />

      <!-- Numeral 2 Structure -->
      <!-- Sweeping curve, diagonal traverse and horizontal base -->
      <path d="M 292,185 C 292,130 330,115 365,115 C 405,115 425,140 425,180 C 425,235 370,295 292,395 L 425,395"
            fill="none" stroke="url(#linkGrad)" stroke-width="28" stroke-linecap="round" stroke-linejoin="round" />
    </g>

    <!-- Network Nodes (Vertices) -->
    <g filter="url(#nodeGlow)">
      <!-- Node 4A (Top diagonal of 4) -->
      <circle cx="165" cy="125" r="14" fill="rgba(255,255,255,0.3)" />
      <circle cx="165" cy="125" r="9" fill="#FFFFFF" />

      <!-- Node 4B (Left corner of 4) -->
      <circle cx="100" cy="265" r="14" fill="rgba(255,255,255,0.3)" />
      <circle cx="100" cy="265" r="9" fill="#FFFFFF" />

      <!-- Node 4C (Intersection of 4) -->
      <circle cx="230" cy="265" r="16" fill="rgba(103, 232, 249, 0.4)" />
      <circle cx="230" cy="265" r="10" fill="#67E8F9" />

      <!-- Node 4D (Bottom stem of 4) -->
      <circle cx="230" cy="395" r="15" fill="rgba(255,255,255,0.3)" />
      <circle cx="230" cy="395" r="9" fill="#FFFFFF" />

      <!-- Node 2A (Crest of 2) -->
      <circle cx="365" cy="115" r="14" fill="rgba(255,255,255,0.3)" />
      <circle cx="365" cy="115" r="9" fill="#FFFFFF" />

      <!-- Node 2B (Right curve vertex) -->
      <circle cx="425" cy="180" r="14" fill="rgba(255,255,255,0.3)" />
      <circle cx="425" cy="180" r="9" fill="#FFFFFF" />

      <!-- Node 2C (Inflection mesh bridge point) -->
      <circle cx="335" cy="305" r="12" fill="rgba(103, 232, 249, 0.4)" />
      <circle cx="335" cy="305" r="7" fill="#67E8F9" />

      <!-- Node 2D (Bottom-left base of 2) -->
      <circle cx="292" cy="395" r="15" fill="rgba(255,255,255,0.3)" />
      <circle cx="292" cy="395" r="9" fill="#FFFFFF" />

      <!-- Node 2E (Bottom-right terminus of 2) -->
      <circle cx="425" cy="395" r="15" fill="rgba(103, 232, 249, 0.4)" />
      <circle cx="425" cy="395" r="9" fill="#67E8F9" />
    </g>
  </g>
</svg>`;
};

async function generate() {
  console.log('Generating Easy42 branding assets...');

  // 1. Write SVGs
  const standardSvg = createLogoSvg({ transparent: false, paddingRatio: 0 });
  const maskableSvg = createLogoSvg({ transparent: false, paddingRatio: 0.1 });
  const transparentSvg = createLogoSvg({ transparent: true, paddingRatio: 0 });

  fs.writeFileSync(path.join(publicDir, 'logo.svg'), standardSvg);
  fs.writeFileSync(path.join(publicDir, 'favicon.svg'), standardSvg);
  fs.writeFileSync(path.join(publicDir, 'logo-transparent.svg'), transparentSvg);

  // 2. Render PNGs using @resvg/resvg-js
  const renderPng = (svgContent, width) => {
    const resvg = new Resvg(svgContent, {
      fitTo: {
        mode: 'width',
        value: width,
      },
    });
    return resvg.render().asPng();
  };

  const sizes = [
    { name: 'favicon-16x16.png', size: 16, svg: standardSvg },
    { name: 'favicon-32x32.png', size: 32, svg: standardSvg },
    { name: 'favicon-48x48.png', size: 48, svg: standardSvg },
    { name: 'apple-touch-icon.png', size: 180, svg: standardSvg },
    { name: 'pwa-192x192.png', size: 192, svg: standardSvg },
    { name: 'pwa-512x512.png', size: 512, svg: standardSvg },
    { name: 'pwa-512x512-maskable.png', size: 512, svg: maskableSvg },
  ];

  const pngBuffers = {};
  for (const item of sizes) {
    const pngBuf = renderPng(item.svg, item.size);
    fs.writeFileSync(path.join(publicDir, item.name), pngBuf);
    pngBuffers[item.name] = pngBuf;
    console.log(`Rendered ${item.name} (${item.size}x${item.size})`);
  }

  // 3. Generate multi-resolution favicon.ico (16, 32, 48)
  const icoBuffer = await pngToIco([
    path.join(publicDir, 'favicon-16x16.png'),
    path.join(publicDir, 'favicon-32x32.png'),
    path.join(publicDir, 'favicon-48x48.png'),
  ]);
  fs.writeFileSync(path.join(publicDir, 'favicon.ico'), icoBuffer);
  console.log('Generated favicon.ico (16x16, 32x32, 48x48)');

  console.log('All assets generated successfully in web/public!');
}

generate().catch((err) => {
  console.error('Asset generation failed:', err);
  process.exit(1);
});
