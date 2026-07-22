import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Kraube API',
  description: 'Lightweight Go gateway for Anthropic Messages API via OAuth subscription',
  base: '/kraube-api/',
  ignoreDeadLinks: [/localhost/],
  appearance: false,
  markdown: {
    theme: {
      name: 'kraube-dark',
      type: 'dark',
      colors: {
        'editor.background': '#2b3140',
        'editor.foreground': '#cbd5e1',
      },
      settings: [
        { scope: ['comment', 'punctuation.definition.comment'], settings: { foreground: '#6b7a8d' } },
        { scope: ['keyword', 'storage', 'keyword.control', 'keyword.operator.expression'], settings: { foreground: '#e2e8f0', fontStyle: 'bold' } },
        { scope: ['string', 'string.quoted'], settings: { foreground: '#8b9dc3' } },
        { scope: ['entity.name.function', 'support.function'], settings: { foreground: '#a8bbd4' } },
        { scope: ['entity.name.type', 'support.type', 'storage.type'], settings: { foreground: '#94a8c4' } },
        { scope: ['variable', 'variable.other', 'variable.parameter'], settings: { foreground: '#cbd5e1' } },
        { scope: ['constant', 'constant.numeric', 'constant.language'], settings: { foreground: '#e2e8f0' } },
        { scope: ['punctuation', 'meta.brace', 'punctuation.separator'], settings: { foreground: '#7b8a9e' } },
        { scope: ['keyword.operator', 'keyword.operator.assignment'], settings: { foreground: '#94a3b8' } },
        { scope: ['entity.name.tag'], settings: { foreground: '#e2e8f0' } },
        { scope: ['entity.other.attribute-name'], settings: { foreground: '#94a8c4' } },
        { scope: ['meta.object-literal.key'], settings: { foreground: '#a8bbd4' } },
        { scope: ['source'], settings: { foreground: '#cbd5e1' } },
      ],
    },
  },
  head: [
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }],
    ['link', { href: 'https://fonts.googleapis.com/css2?family=Alegreya+Sans+SC:wght@400;500;700;800&family=Inter:wght@400;500;600;700&display=swap', rel: 'stylesheet' }],
  ],
  themeConfig: {
    logo: '/logo.png',
    siteTitle: 'Kraube API',
    nav: [
      { text: 'Guide', link: '/guide/getting-started' },
      { text: 'Reference', link: '/reference/api' },
      { text: 'GitHub', link: 'https://github.com/scott-walker/kraube-api' },
    ],
    sidebar: {
      '/guide/': [
        {
          text: 'Introduction',
          items: [
            { text: 'What is Kraube API?', link: '/guide/what-is-kraube-api' },
            { text: 'Getting Started', link: '/guide/getting-started' },
          ],
        },
        {
          text: 'Core Concepts',
          items: [
            { text: 'TokenProvider', link: '/guide/token-provider' },
            { text: 'Authentication', link: '/guide/authentication' },
            { text: 'Streaming', link: '/guide/streaming' },
          ],
        },
        {
          text: 'Features',
          items: [
            { text: 'Tool Use', link: '/guide/tool-use' },
            { text: 'Extended Thinking', link: '/guide/thinking' },
            { text: 'Vision & Documents', link: '/guide/vision' },
            { text: 'Structured Output', link: '/guide/structured-output' },
            { text: 'Serve Daemon', link: '/guide/serve' },
          ],
        },
        {
          text: 'Advanced',
          items: [
            { text: 'Architecture', link: '/guide/architecture' },
            { text: 'Protocol Details', link: '/guide/protocol' },
            { text: 'Contributing', link: '/guide/contributing' },
          ],
        },
      ],
      '/reference/': [
        {
          text: 'Reference',
          items: [
            { text: 'API Coverage', link: '/reference/api' },
            { text: 'Options', link: '/reference/options' },
            { text: 'CLI', link: '/reference/cli' },
            { text: 'Changelog', link: '/reference/changelog' },
          ],
        },
      ],
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/scott-walker/kraube-api' },
    ],
    search: {
      provider: 'local',
    },
    footer: {
      message: 'Released under the MIT License.',
    },
  },
})
