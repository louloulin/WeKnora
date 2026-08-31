<!--
  CollabSlidesView — v0.7.37 Build #44 / v0.7.38 Build #46.x.

  Wraps CollabSlideDeckEditor with auth + tenant context. Higher-level
  "演示文稿" surface distinct from per-page collab-doc editor.
-->
<template>
  <div class="collab-slides-view">
    <CollabSlideDeckEditor />
  </div>
</template>

<script setup lang="ts">
import CollabSlideDeckEditor from '@/components/collab/CollabSlideDeckEditor.vue'
</script>

<style scoped>
.collab-slides-view { padding: 0; }
</style>
