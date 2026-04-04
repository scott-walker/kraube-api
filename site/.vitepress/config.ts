import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Kraube API',
  description: 'Lightweight Go gateway for Anthropic Messages API via OAuth subscription',
  base: '/kraube-api/',
  ignoreDeadLinks: [/localhost/],
  appearance: false,
  head: [
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }],
    ['link', { href: 'https://fonts.googleapis.com/css2?family=Alegreya+Sans+SC:wght@400;500;700;800&family=Inter:wght@400;500;600;700&display=swap', rel: 'stylesheet' }],
  ],
  themeConfig: {
    logo: undefined,
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
