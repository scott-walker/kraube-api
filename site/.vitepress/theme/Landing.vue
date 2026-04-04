<template>
  <div class="landing">
    <div class="landing-container">
      <div class="hero">
        <img :src="withBase('/brand-int-text-2.png')" alt="Kraube API" class="brand-logo">
        <p class="tagline">Lightweight Go gateway for Anthropic Messages API via OAuth subscription.</p>
      </div>

      <div class="install">
        <div class="code-block" @click="copy" :title="copied ? 'Copied!' : 'Click to copy'">
          <span class="comment"># install</span><br>
          <span class="prompt">$ </span>go get github.com/scott-walker/kraube-api<br>
          <br>
          <span class="comment"># authenticate</span><br>
          <span class="prompt">$ </span>kraube login
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
