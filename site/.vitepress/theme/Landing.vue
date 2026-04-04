<template>
  <div class="landing">
    <div class="landing-container">
      <div class="hero">
        <img :src="withBase('/brand-int.svg')" alt="Kraube API" class="brand-logo">
        <p class="tagline">Lightweight Go gateway for Anthropic Messages API via OAuth subscription.</p>
      </div>

      <div class="install">
        <div class="code-block">
          <button class="copy-btn" @click="copy">
            <svg v-if="!copied" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="8" height="4" x="8" y="2" rx="1" ry="1"/><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/></svg>
            <svg v-else xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 14 2 2 4-4"/><rect width="8" height="4" x="8" y="2" rx="1" ry="1"/><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/></svg>
          </button>
          <span class="comment"># install</span><br>
          <span class="prompt">$ </span><span class="cmd">go</span> get github.com/scott-walker/kraube-api<br>
          <br>
          <span class="comment"># authenticate</span><br>
          <span class="prompt">$ </span><span class="cmd">kraube</span> login
        </div>
      </div>

      <div class="features">
        <div class="feature">
          <div class="feature-title">OAuth-only</div>
          <div class="feature-desc">Access Claude through your Pro/Max/Team subscription. No API key needed.</div>
        </div>
        <div class="feature">
          <div class="feature-title">Stateless</div>
          <div class="feature-desc">Token from anywhere via TokenProvider. File, env, Vault, callback.</div>
        </div>
        <div class="feature">
          <div class="feature-title">Lightweight</div>
          <div class="feature-desc">Minimal HTTP wrapper. Not a framework. Just types and functions.</div>
        </div>
        <div class="feature">
          <div class="feature-title">Full coverage</div>
          <div class="feature-desc">Streaming, tool use, thinking, vision, documents, structured output.</div>
        </div>
      </div>

      <div class="links">
        <a href="https://github.com/scott-walker/kraube-api">GitHub</a>
        <a :href="withBase('/guide/getting-started')">Get Started</a>
        <a :href="withBase('/reference/api')">API Reference</a>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { withBase } from 'vitepress'
const copied = ref(false)
function copy() {
  navigator.clipboard.writeText('go get github.com/scott-walker/kraube-api')
  copied.value = true
  setTimeout(() => copied.value = false, 2000)
}
</script>
