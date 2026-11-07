import { defineConfig } from 'vitepress'

const repo = 'https://github.com/vitoramaral10/cloudflare-mcp'

export default defineConfig({
  title: 'cloudflare-mcp',
  description: 'The whole Cloudflare API as MCP tools, and a search that finds the right one. An unofficial MCP server in one Go binary.',
  // Served from GitHub Pages under the repository's name.
  base: '/cloudflare-mcp/',
  cleanUrls: true,
  lastUpdated: true,
  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/cloudflare-mcp/logo.svg' }],
    ['meta', { name: 'theme-color', content: '#5b6cf0' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:title', content: 'cloudflare-mcp: the whole Cloudflare API as MCP tools' }],
    ['meta', { property: 'og:description', content: 'One tool per Cloudflare API operation, plus find and call: a search that picks the right operation, even in Portuguese.' }],
    ['meta', { property: 'og:image', content: 'https://vitoramaral10.github.io/cloudflare-mcp/social-preview.png' }],
  ],
  themeConfig: {
    logo: '/logo.svg',
    nav: [
      { text: 'Guide', link: '/getting-started', activeMatch: '^/(getting-started|usage)' },
      { text: 'Reference', link: '/tools', activeMatch: '^/(tools|catalog|search|operations|architecture)' },
      { text: 'Changelog', link: `${repo}/blob/main/CHANGELOG.md` },
    ],
    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Getting started', link: '/getting-started' },
          { text: 'Using the tools', link: '/usage' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Tools', link: '/tools' },
          { text: 'Operation catalog', link: '/catalog' },
          { text: 'How find works', link: '/search' },
          { text: 'Running in production', link: '/operations' },
          { text: 'Architecture', link: '/architecture' },
        ],
      },
      {
        text: 'Project',
        items: [
          { text: 'Contributing', link: `${repo}/blob/main/CONTRIBUTING.md` },
          { text: 'Security', link: `${repo}/blob/main/SECURITY.md` },
          { text: 'Changelog', link: `${repo}/blob/main/CHANGELOG.md` },
        ],
      },
    ],
    socialLinks: [{ icon: 'github', link: repo }],
    search: { provider: 'local' },
    outline: { level: [2, 3] },
    editLink: { pattern: `${repo}/edit/main/docs/:path`, text: 'Edit this page on GitHub' },
    footer: {
      message: 'Released under the MIT License. Not affiliated with Cloudflare, Inc.',
      copyright: 'Copyright © 2026 Vitor Amaral',
    },
  },
})
