<!--
  CollabSlideKonvaEditor — v0.7.27 飞书级 PPT 形状编辑器。

  Architecture:
   1. Open: GET /collaborative-docs/:id/download (latest .pptx bytes)
            -> pptxShapeAdapter.openPptxShapes(bytes) parses shapes (text,
            rect, ellipse, line, picture) from each slide.
   2. Realtime: Y.Array<Y.Map<shape>> keyed by slide index. Two clients
            editing different shapes converge via Yjs CRDT.
   3. Save: pptxShapeAdapter.savePptxShapeBytes(deck) emits .pptx bytes
            via pptx-engine; unchanged slides stay byte-identical.
   4. Drag/transform: Konva.Transformer handles resize/move; dragend
            commits new (x, y, w, h) into the per-shape Y.Map.

  Coverage today: text, rect, ellipse, line, picture. PPT charts, tables,
  SmartArt, and 3D shapes render read-only (their bytes survive the
  round-trip verbatim; only the shapes this editor touches are regenerated).
-->
<template>
  <div class="collab-slide-konva" :data-rb-theme="ribbonTheme">
    <header class="collab-slide-konva__titlebar">
      <div class="collab-slide-konva__brandmark">W</div>
      <div class="collab-slide-konva__file-meta">
        <div class="collab-slide-konva__title">{{ title }}</div>
        <div class="collab-slide-konva__file-subtitle"><span>{{ kindLabel }}</span><span class="collab-slide-konva__file-dot">·</span><span>云端演示文稿</span></div>
      </div>
      <div class="collab-slide-konva__title-actions">
        <span class="collab-slide-konva__connection" :class="{ connected: connected && !saveError }"><i></i>{{ connectionLabel }}</span>
        <span class="collab-slide-konva__savetag" :class="savetagClass">{{ saveLabel }}</span>
        <span class="collab-slide-konva__peers" aria-label="协作者">
          <span v-for="p in peers" :key="p.clientId" class="collab-slide-konva__peer" :style="{ backgroundColor: p.color }" :title="p.displayName">{{ initialOf(p.displayName) }}</span>
        </span>
        <button class="collab-slide-konva__title-btn" type="button" @click="onEnterPresent" :disabled="!slides.length || loading" data-tip="从当前页开始演示 (F5)"><span>▶</span> 演示</button>
      </div>
    </header>
    <CollabEditorRibbon
      v-model="activeTab"
      :tabs="ribbonTabs"
      aria-label="演示文稿工具栏"
      test-id-prefix="slide-ribbon"
      storage-key="weknora-slide-ribbon-collapsed"
      collapsible
      :theme="ribbonTheme"
    >
      <template #default>
        <div class="collab-slide-konva__ribbon-groups">
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'home'">
            <div class="ribbon-group-items">
              <!-- v0.7.139 — AI entry group (GenOffice pattern). The AI panel
               * already exists (CollabAiPolishDialog). Surface the most
               * common actions inline so the home tab feels as dense as
               * GenOffice's image-2.png reference. -->
              <button
                class="rb-big rb-big-ai"
                type="button"
                :disabled="!slides.length || loading"
                data-testid="slide-ai-panel"
                data-tip="AI 助手：润色 / 重写 / 提问当前幻灯片"
                @click="onOpenAiPanel"
              >
                <span class="rb-big-icon">
                  <CollabIcon name="IconSparkles" />
                </span>
                <span>AI 助手</span>
              </button>
              <button
                class="rb-big rb-big-ai"
                type="button"
                :disabled="!selectedId || loading"
                data-testid="slide-ai-polish"
                data-tip="AI 润色当前选中文本"
                @click="onAiPolishSelected"
              >
                <span class="rb-big-icon">
                  <CollabIcon name="IconWand" />
                </span>
                <span>AI 润色</span>
              </button>
              <button
                class="rb-big rb-big-ai"
                type="button"
                :disabled="!selectedId || loading"
                data-testid="slide-ai-suggest"
                data-tip="AI 为当前幻灯片生成建议"
                @click="onAiSuggestSlide"
              >
                <span class="rb-big-icon">
                  <CollabIcon name="IconAiPanel" />
                </span>
                <span>AI 建议</span>
              </button>
              <button
                class="rb-big rb-big-ai"
                type="button"
                :disabled="!selectedId || loading"
                data-testid="slide-ai-rewrite"
                data-tip="AI 改写选中文本"
                @click="onAiRewrite"
              >
                <span class="rb-big-icon">
                  <CollabIcon name="IconAiAsk" />
                </span>
                <span>AI 改写</span>
              </button>
              <button
                class="rb-big rb-big-ai"
                type="button"
                :disabled="!slides.length || loading"
                data-testid="slide-ai-image"
                data-tip="AI 为当前幻灯片配图"
                @click="onAiImage"
              >
                <span class="rb-big-icon">
                  <CollabIcon name="IconAiImage" />
                </span>
                <span>AI 配图</span>
              </button>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">AI 工具</div>
          </div>
          <div class="ribbon-sep" v-if="activeTab === 'home'" />
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'home'">
            <div class="ribbon-group-items">
              <!-- v0.7.139 — Clipboard group (GenOffice pattern). Paste big btn
               * + stacked cut/copy/duplicate icons. Adds visual density
               * matching the reference ribbon. -->
              <button
                class="rb-big"
                type="button"
                :disabled="!canPaste"
                data-testid="slide-paste"
                data-tip="粘贴 (Ctrl+V)"
                @click="onPasteSelected"
              >
                <span class="rb-big-icon">
                  <CollabIcon name="IconPaste" />
                </span>
                <span>粘贴</span>
              </button>
              <div class="rb-col rb-clip-col">
                <button
                  class="rb-icon"
                  type="button"
                  :disabled="!selectedId"
                  data-tip="剪切 (Ctrl+X)"
                  data-testid="slide-cut"
                  @click="onCutSelected"
                >
                  <CollabIcon name="IconCut" />
                </button>
                <button
                  class="rb-icon"
                  type="button"
                  :disabled="!selectedId"
                  data-tip="复制 (Ctrl+C)"
                  data-testid="slide-copy"
                  @click="onCopySelected"
                >
                  <CollabIcon name="IconCopy" />
                </button>
                <button
                  class="rb-icon"
                  type="button"
                  :disabled="!selectedId"
                  data-tip="格式刷：复制格式并应用"
                  data-testid="slide-format-painter"
                  @click="onFormatPainter"
                >
                  <CollabIcon name="IconFormatPainter" />
                </button>
              </div>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">剪贴板</div>
          </div><div class="ribbon-sep" v-if="activeTab === 'home' && !playCollapsed" />
          <!-- v0.7.145 — Play group with GenOffice collapse pattern -->
          <div v-if="activeTab === 'home'">
            <div v-if="playCollapsed" class="collab-slide-konva__tool-group ribbon-group ribbon-group--collapsed">
              <div class="ribbon-group-items">
                <div class="rb-drop-wrap">
                  <button
                    class="rb-big"
                    type="button"
                    data-tip="放映：演示 / 撤销 / 重做"
                    data-testid="slide-play-collapse"
                    @click="togglePlayCollapse"
                  >
                    <span class="rb-big-icon"><CollabIcon name="IconPlayFromStart" /><svg class="rb-caret" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M5.5 9.25 12 15.75l6.5-6.5" stroke="currentColor" stroke-width="2.6" strokeLinecap="round" strokeLinejoin="round" /></svg></span>
                    <span>放映</span>
                  </button>
                </div>
              </div>
            </div>
            <div v-else class="collab-slide-konva__tool-group ribbon-group">
              <div class="ribbon-group-items">
                <!-- Show group: split-button 演示 + 撤销 / 重做 (kept compact) -->
              <div class="rb-drop-wrap">
                <button
                  class="rb-big rb-split rb-show-split"
                  :class="{ 'is-open': slideShowOpen }"
                  type="button"
                  :disabled="!slides.length || loading"
                  data-testid="slide-present-btn"
                  :data-tip="slideShowFromStart ? '从第一页开始演示 (F5)' : '从当前页开始演示 (Shift+F5)'"
                  @click="onPresentSplitMain"
                >
                  <span class="rb-big-icon">
                    <span class="rb-split-main"><CollabIcon :name="slideShowFromStart ? 'IconPlayFromStart' : 'IconPlayCurrent'" /></span>
                    <span class="rb-caret-hit" data-tip="演示选项" @click.stop="slideShowOpen = !slideShowOpen; layoutOpen = false; layoutPickOpen = false"><svg class="rb-caret" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M5.5 9.25 12 15.75l6.5-6.5" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" /></svg></span>
                  </span>
                  <span>{{ slideShowFromStart ? '从开始' : '从当前页' }}</span>
                </button>
                <div v-if="slideShowOpen" class="rb-drop" data-testid="slide-present-menu" @click.stop @keydown.escape="slideShowOpen = false" role="menu">
                  <div class="rb-drop-title">演示选项</div>
                  <button type="button" class="rb-drop-item" :class="{ active: slideShowFromStart }" data-testid="slide-present-from-start" @click="slideShowFromStart = true; slideShowOpen = false; onPresentSplitMain">
                    <span>从第一页开始</span><span class="rb-drop-meta">F5</span>
                  </button>
                  <button type="button" class="rb-drop-item" :class="{ active: !slideShowFromStart }" data-testid="slide-present-from-current" @click="slideShowFromStart = false; slideShowOpen = false; onPresentSplitMain">
                    <span>从当前页开始</span><span class="rb-drop-meta">Shift+F5</span>
                  </button>
                </div>
              </div>
              <div class="rb-slides-col">
                <button class="rb-small" type="button" :disabled="!canUndo" data-tip="撤销 (Ctrl+Z)" @click="onUndo"><CollabIcon name="IconRotateLeft" /><span>撤销</span></button>
                <button class="rb-small" type="button" :disabled="!canRedo" data-tip="重做 (Ctrl+Shift+Z)" @click="onRedo"><CollabIcon name="IconRotateRight" /><span>重做</span></button>
                <button class="rb-icon rb-collapse-toggle" type="button" data-tip="折叠" @click="togglePlayCollapse">
                  <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 6h10M5 9h6" /></svg>
                </button>
              </div>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">放映</div>
          </div>
          </div>
          
          <div class="ribbon-sep" v-if="activeTab === 'home'" />
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'home'">
            <div class="ribbon-group-items">
              <!-- v0.7.139 — Font group (GenOffice pattern). Font family picker
               * + font size picker + B/I/U + Aa (case). Mirrors GenOffice's
               * two-row font group. -->
              <div class="rb-font-stack">  <!-- column flex (GenOffice .rb-col): 2 horizontal rows -->
                <div class="rb-font-row rb-font-row--name">
                  <div class="rb-drop-wrap rb-font-family">
                    <button
                      class="rb-small rb-font-btn"
                      type="button"
                      :disabled="!selectedId"
                      data-testid="slide-font-family"
                      data-tip="字体"
                      @click="fontPickerOpen = !fontPickerOpen"
                    >
                      <span class="rb-font-name">{{ selectedFontLabel }}</span>
                      <svg class="rb-caret" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M5.5 9.25 12 15.75l6.5-6.5" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
                    </button>
                  </div>
                </div>
                <div class="rb-font-row rb-font-row--ctrl">  <!-- row 2 horizontal: size picker + grow/shrink -->
                  <div class="rb-drop-wrap rb-font-size">
                    <button
                      class="rb-small rb-font-btn rb-font-btn--size"
                      type="button"
                      :disabled="!selectedId"
                      data-testid="slide-font-size"
                      data-tip="字号"
                      @click="fontSizePickerOpen = !fontSizePickerOpen"
                    >
                      <span class="rb-font-name">{{ selectedShape?.fontSize ?? 18 }}</span>
                      <svg class="rb-caret" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M5.5 9.25 12 15.75l6.5-6.5" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
                    </button>
                  </div>
                  <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="增大字号" data-testid="slide-font-grow" @click="bumpFontSize(2)"><CollabIcon name="IconGrowFont" /></button>
                  <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="减小字号" data-testid="slide-font-shrink" @click="bumpFontSize(-2)"><CollabIcon name="IconShrinkFont" /></button>
                </div>
              </div>
            </div>
            <div class="rb-arrange-row rb-font-format-row">
              <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="加粗 (Ctrl+B)" data-testid="slide-font-bold" @click="toggleBold"><CollabIcon name="IconBold" /></button>
              <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="斜体 (Ctrl+I)" data-testid="slide-font-italic" @click="toggleItalic"><CollabIcon name="IconItalic" /></button>
              <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="下划线 (Ctrl+U)" data-testid="slide-font-underline" @click="toggleUnderline"><CollabIcon name="IconUnderline" /></button>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">字体</div>
          </div>
          <div class="ribbon-sep" v-if="activeTab === 'home'" />
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'home'">
            <div class="ribbon-group-items">
              <!-- v0.7.139 — Paragraph group (GenOffice pattern). 4 alignment
               * icons + bullets + line spacing. Mirrors GenOffice's
               * compact paragraph cluster. -->
              <div class="rb-arrange-row">
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="左对齐" @click="alignText('left')"><CollabIcon name="IconAlignLeft" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="水平居中" @click="alignText('center')"><CollabIcon name="IconAlignCenter" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="右对齐" @click="alignText('right')"><CollabIcon name="IconAlignRight" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="两端对齐" @click="alignText('justify')"><CollabIcon name="IconAlignJustify" /></button>
              </div>
              <div class="rb-arrange-row">
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="项目符号" data-testid="slide-bullets" @click="toggleBullets"><CollabIcon name="IconBullets" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="编号列表" data-testid="slide-numbered" @click="toggleNumbered"><CollabIcon name="IconNumbered" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="增加缩进" data-testid="slide-indent-inc" @click="bumpIndent(1)"><CollabIcon name="IconIndentInc" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="减少缩进" data-testid="slide-indent-dec" @click="bumpIndent(-1)"><CollabIcon name="IconIndentDec" /></button>
              </div>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">段落</div>
          </div>
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'home'">
            <div class="ribbon-group-items">
              <button
                v-for="tool in drawingTools"
                :key="tool.type"
                class="rb-big"
                type="button"
                :disabled="!slides.length || loading"
                :data-testid="tool.testId"
                :data-tip="tool.label"
                @click="addShape(tool.type)"
              >
                <span class="rb-big-icon"><CollabIcon :name="tool.icon" /></span>
                <span>{{ tool.label }}</span>
              </button>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">绘图</div>
          </div>

          <div class="ribbon-sep" v-if="activeTab === 'home'" />
          <div class="collab-slide-konva__tool-group ribbon-group ribbon-group--collapsed" v-if="activeTab === 'home' && slidesCollapsed && !slidesPanelOpen">
            <div class="ribbon-group-items">
              <div class="rb-drop-wrap">
                <button
                  class="rb-big"
                  :class="{ active: slidesPanelOpen }"
                  type="button"
                  :disabled="!slides.length || loading"
                  data-testid="slide-slides-collapse"
                  @click.stop="toggleSlidesPanel"
                >
                  <span class="rb-big-icon">
                    <CollabIcon name="IconNewSlide" />
                    <svg class="rb-caret" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M5.5 9.25 12 15.75l6.5-6.5" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
                  </span>
                  <span>幻灯片</span>
                </button>
              </div>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">幻灯片</div>
          </div>

          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'home' && (!slidesCollapsed || slidesPanelOpen)">
            <div class="ribbon-group-items">
              <!-- Slides group: split-button 新建幻灯片 + stacked 布局 / 添加节 -->
              <div class="rb-drop-wrap">
                <button
                  class="rb-big rb-split"
                  :class="{ 'is-open': layoutOpen }"
                  type="button"
                  :disabled="!slides.length || loading"
                  data-testid="slide-add-slide-btn"
                  data-tip="新建空白幻灯片"
                  @click="addSlide"
                >
                  <span class="rb-big-icon">
                    <span class="rb-split-main"><CollabIcon name="IconNewSlide" /></span>
                    <span class="rb-caret-hit" data-tip="选择布局新建" @click.stop="toggleLayoutPicker"><svg class="rb-caret" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M5.5 9.25 12 15.75l6.5-6.5" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" /></svg></span>
                  </span>
                  <span>新建幻灯片</span>
                </button>
                <div v-if="layoutOpen" class="rb-drop rb-layout-panel" data-testid="slide-layout-picker" @click.stop @keydown.escape="layoutOpen = false" role="menu">
                  <div class="rb-drop-title">选择布局新建</div>
                  <button
                    v-for="layout in availableLayouts"
                    :key="layout.path"
                    type="button"
                    class="rb-drop-item"
                    :data-testid="`slide-layout-opt-${layout.path}`"
                    @click="applyLayoutFromSplit(layout.path)"
                  >
                    <span>{{ layout.name }}</span>
                    <span class="rb-drop-meta">{{ layout.placeholders }} 个占位符</span>
                  </button>
                  <div v-if="missingBuiltins.length" class="rb-drop-sep">内置布局</div>
                  <button
                    v-for="b in missingBuiltins"
                    :key="b.key"
                    type="button"
                    class="rb-drop-item"
                    :data-testid="`slide-layout-builtin-${b.key}`"
                    @click="applyLayoutFromSplit('builtin:' + b.key)"
                  >
                    <span>{{ b.name }}</span>
                    <span class="rb-drop-meta">内置</span>
                  </button>
                  <div v-if="!availableLayouts.length && !missingBuiltins.length" class="rb-drop-empty">该模板未声明布局</div>
                </div>
              </div>
              <div class="rb-slides-col">
                <div class="rb-drop-wrap">
                  <button
                    class="rb-small"
                    :class="{ active: layoutPickOpen }"
                    type="button"
                    :disabled="!slides.length || loading"
                    data-testid="slide-layout-btn"
                    data-tip="切换当前幻灯片布局"
                    @click="toggleLayoutPick"
                  >
                    <CollabIcon name="IconTheme" />
                    <span>布局</span>
                    <svg class="rb-caret" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M5.5 9.25 12 15.75l6.5-6.5" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
                  </button>
                  <div v-if="layoutPickOpen" class="rb-drop" data-testid="slide-layout-menu" @click.stop @keydown.escape="layoutPickOpen = false" role="menu">
                    <div class="rb-drop-title">更改当前页布局</div>
                    <button v-for="layout in availableLayouts" :key="layout.path" type="button" class="rb-drop-item" @click="applyLayout(layout.path); layoutPickOpen = false">
                      <span>{{ layout.name }}</span><span class="rb-drop-meta">{{ layout.placeholders }} 个占位符</span>
                    </button>
                    <div v-if="missingBuiltins.length" class="rb-drop-sep">内置布局</div>
                    <button v-for="b in missingBuiltins" :key="b.key" type="button" class="rb-drop-item" @click="applyLayout('builtin:' + b.key); layoutPickOpen = false">
                      <span>{{ b.name }}</span><span class="rb-drop-meta">内置</span>
                    </button>
                    <div class="rb-drop-sep" />
                    <button type="button" class="rb-drop-item rb-drop-item--muted" @click="resetSlideLayout(); layoutPickOpen = false"><span>重置为占位符布局</span></button>
                  </div>
                </div>
                <button class="rb-small" type="button" :disabled="!slides.length" data-tip="添加节" @click="onAddSection">
                  <CollabIcon name="IconAddSection" /><span>添加节</span>
                </button>
              </div>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">幻灯片</div>
          </div>
          <div class="ribbon-sep" v-if="activeTab === 'home' || activeTab === 'insert'" />
          <!-- v0.7.145 — GenOffice collapse pattern: when narrow, show one button + dropdown. -->
          <div v-if="activeTab === 'home' || activeTab === 'insert'">
            <div v-if="insertCollapsed" class="collab-slide-konva__tool-group ribbon-group ribbon-group--collapsed">
              <div class="ribbon-group-items">
                <div class="rb-drop-wrap">
                  <button
                    class="rb-big"
                    type="button"
                    data-tip="插入：文本框 / 矩形 / 表格 / 导入 PPTX"
                    data-testid="slide-insert-collapse"
                    @click="toggleInsertCollapse"
                  >
                    <span class="rb-big-icon"><CollabIcon name="IconInsert" /><svg class="rb-caret" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M5.5 9.25 12 15.75l6.5-6.5" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" /></svg></span>
                    <span>插入</span>
                  </button>
                </div>
              </div>
            </div>
            <div v-else class="collab-slide-konva__tool-group ribbon-group">
              <div class="ribbon-group-items">
                <button class="rb-big" @click="addShape('text')" type="button" data-testid="slide-add-text" data-tip="插入文本框"><span class="rb-big-icon"><CollabIcon name="IconTextBox" /></span><span>文本框</span></button>
                <button class="rb-big" @click="addShape('rect')" type="button" data-testid="slide-add-rect" data-tip="插入矩形"><span class="rb-big-icon"><CollabIcon name="IconRectangle" /></span><span>矩形</span></button>
                <button class="rb-big" @click="promptAddTable" type="button" data-tip="插入表格" data-testid="slide-add-table"><span class="rb-big-icon"><CollabIcon name="IconTable" /></span><span>表格</span></button>
                <button class="rb-big" @click="triggerUpload" type="button" :disabled="uploading" data-tip="导入本地 PPTX"><span class="rb-big-icon"><CollabIcon name="IconUpload" /></span><span>{{ uploading ? '上传中…' : '导入 PPTX' }}</span></button>
                <button class="rb-icon rb-collapse-toggle" type="button" data-tip="折叠" @click="toggleInsertCollapse">
                  <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 6h10M5 9h6" /></svg>
                </button>
              </div>
              <div class="ribbon-group-label ribbon-group-label--visible">插入</div>
            </div>
          </div>
          <div class="ribbon-sep" v-if="activeTab === 'home' && !arrangeCollapsed" />
          <!-- v0.7.145 — Arrange group with GenOffice collapse pattern -->
          <div v-if="activeTab === 'home'">
            <div v-if="arrangeCollapsed" class="collab-slide-konva__tool-group ribbon-group ribbon-group--collapsed">
              <div class="ribbon-group-items">
                <div class="rb-drop-wrap">
                  <button
                    class="rb-big"
                    type="button"
                    data-tip="排列：对齐 / 分布 / 翻转 / 组合"
                    data-testid="slide-arrange-collapse"
                    @click="toggleArrangeCollapse"
                  >
                    <span class="rb-big-icon"><CollabIcon name="IconArrangeAll" /><svg class="rb-caret" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M5.5 9.25 12 15.75l6.5-6.5" stroke="currentColor" stroke-width="2.6" strokeLinecap="round" stroke-linejoin="round" /></svg></span>
                    <span>排列</span>
                  </button>
                </div>
              </div>
            </div>
            <div v-else class="collab-slide-konva__tool-group ribbon-group">
              <div class="ribbon-group-items">
                <!-- Arrange group: 2×3 align grid + distribute + group + flip row -->
              <div class="rb-arrange-grid" role="group" aria-label="对齐">
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="左对齐" @click="alignSelected('left')"><CollabIcon name="IconObjAlignLeft" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="水平居中" @click="alignSelected('centerH')"><CollabIcon name="IconObjAlignCenterH" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="右对齐" @click="alignSelected('right')"><CollabIcon name="IconObjAlignRight" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="顶端对齐" @click="alignSelected('top')"><CollabIcon name="IconObjAlignTop" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="垂直居中" @click="alignSelected('centerV')"><CollabIcon name="IconObjAlignMiddle" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="底端对齐" @click="alignSelected('bottom')"><CollabIcon name="IconObjAlignBottom" /></button>
              </div>
              <div class="rb-arrange-row">
                <button class="rb-icon" type="button" :disabled="selectedIds.length < 3" data-tip="横向均匀分布" @click="distributeSelected('h')"><CollabIcon name="IconObjDistributeH" /></button>
                <button class="rb-icon" type="button" :disabled="selectedIds.length < 3" data-tip="纵向均匀分布" @click="distributeSelected('v')"><CollabIcon name="IconObjDistributeV" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="水平翻转" @click="flipSelected('h')"><CollabIcon name="IconObjectFlipH" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="垂直翻转" @click="flipSelected('v')"><CollabIcon name="IconObjectFlipV" /></button>
                <button class="rb-icon" type="button" :disabled="!canGroupSelected" data-tip="组合选中的形状" data-testid="slide-group" @click="groupSelected"><CollabIcon name="IconGroup" /></button>
                <button class="rb-icon" type="button" :disabled="!selectedId" data-tip="复制所选" @click="duplicateSelected"><CollabIcon name="IconCopy" /></button>
                <button class="rb-icon rb-collapse-toggle" type="button" data-tip="折叠" @click="toggleArrangeCollapse">
                  <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 6h10M5 9h6" /></svg>
                </button>
              </div>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">排列</div>
          </div>
          </div>
          <div class="ribbon-sep" v-if="activeTab === 'insert' || activeTab === 'draw'" />
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'insert' || activeTab === 'draw'">
            <div class="ribbon-group-items">
              <button class="rb-big" v-for="shapeTool in shapeTools" :key="shapeTool.type" @click="addShape(shapeTool.type)" type="button" :data-testid="shapeTool.testId" :data-tip="shapeTool.label"><span class="rb-big-icon"><CollabIcon :name="shapeTool.icon" /></span><span>{{ shapeTool.label }}</span></button>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">形状</div>
          </div>
          <div class="ribbon-sep" v-if="activeTab === 'design'" />
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'design'">
            <div class="ribbon-group-items">
              <button
              type="button"
              class="rb-big collab-slide-konva__layout-btn"
              data-testid="slide-layout-btn"
              :class="{ 'is-open': layoutMenuOpen }"
              :disabled="!slides.length || loading"
              data-tip="切换幻灯片布局"
              @click="layoutMenuOpen = !layoutMenuOpen; $event.stopPropagation()"
            >
              <span class="rb-big-icon is-with-caret">
                <CollabIcon name="IconTheme" />
                <svg class="rb-caret" width="10" height="10" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path d="M5.5 9.25 12 15.75l6.5-6.5" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" />
                </svg>
              </span>
              <span>布局</span>
            </button>
            <div v-if="layoutMenuOpen" class="collab-slide-konva__layout-menu" @click.stop role="menu" @keydown.escape="layoutMenuOpen = false" data-testid="slide-layout-menu">
              <div class="collab-slide-konva__layout-menu-title">选择幻灯片布局</div>
              <button
                v-for="layout in availableLayouts"
                :key="layout.path"
                type="button"
                class="collab-slide-konva__layout-menu-item"
                :data-testid="`slide-layout-opt-${layout.path}`"
                @click="applyLayout(layout.path); layoutMenuOpen = false"
              >
                <span class="collab-slide-konva__layout-menu-name">{{ layout.name }}</span>
                <span class="collab-slide-konva__layout-menu-meta">{{ layout.placeholders }} 个占位符</span>
              </button>
              <div v-if="missingBuiltins.length" class="collab-slide-konva__layout-menu-sep">内置布局</div>
              <button
                v-for="b in missingBuiltins"
                :key="b.key"
                type="button"
                class="collab-slide-konva__layout-menu-item collab-slide-konva__layout-menu-item--builtin"
                :data-testid="`slide-layout-builtin-${b.key}`"
                @click="applyLayout('builtin:' + b.key); layoutMenuOpen = false"
              >
                <span class="collab-slide-konva__layout-menu-name">{{ b.name }}</span>
                <span class="collab-slide-konva__layout-menu-meta">内置</span>
              </button>
              <div v-if="!availableLayouts.length && !missingBuiltins.length" class="collab-slide-konva__layout-menu-empty">该模板未声明布局</div>
            </div>
              <CollabSlideThemePanel class="collab-slide-konva__theme-panel" @theme:apply="onThemePanelApply" />
            </div>
          </div>
          <div class="ribbon-sep" v-if="activeTab === 'transitions'" />
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'transitions'">
            <div class="ribbon-group-items">
              <button class="rb-big" type="button" data-testid="slide-tab-transitions-open" data-tip="打开底部切换效果面板" @click="onOpenAnimationsPanel">
                <span class="rb-big-icon">⤵</span>
                <span>打开切换面板</span>
              </button>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">转场效果</div>
          </div>
          <div class="ribbon-sep" v-if="activeTab === 'animate'" />
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'animate'">
            <div class="ribbon-group-items">
              <button class="rb-big" type="button" data-testid="slide-tab-animate-open" data-tip="打开底部动画面板" @click="onOpenAnimationsPanel">
                <span class="rb-big-icon">▶</span>
                <span>打开动画面板</span>
              </button>
            </div>
            <div class="ribbon-group-label ribbon-group-label--visible">对象动画</div>
          </div>
          <div class="ribbon-sep" v-if="activeTab === 'slideshow'" />
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'slideshow'">
            <div class="ribbon-group-items">
              <button class="rb-big rb-big--present" type="button" data-testid="slide-present-btn" :disabled="!slides.length || loading" @click="onEnterPresent" data-tip="全屏演示 (F5)"><span class="rb-big-icon"><CollabIcon name="IconPlay" /></span><span>从当前页</span></button>
              <button class="rb-big" @click="onDownload" type="button" :disabled="downloading" data-tip="导出为 PPTX 文件"><span class="rb-big-icon"><CollabIcon name="IconDownload" /></span><span>{{ downloading ? '下载中…' : '导出 PPTX' }}</span></button>
            </div>
          </div>
          <div class="ribbon-sep" v-if="activeTab === 'review'" />
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'review'">
            <span class="collab-slide-konva__ribbon-hint">评论面板已固定在编辑区右侧，可直接协作讨论。</span>
          </div>
          <div class="ribbon-sep" v-if="activeTab === 'view'" />
          <div class="collab-slide-konva__tool-group ribbon-group" v-if="activeTab === 'view'">
            <div class="ribbon-group-items">
              <button class="rb-small" type="button" @click="onZoomOut" data-tip="缩小画布"><CollabIcon name="IconZoomOut" /><span>缩小</span></button>
              <button class="rb-small" type="button" @click="onZoomIn" data-tip="放大画布"><CollabIcon name="IconZoomIn" /><span>放大</span></button>
              <button class="rb-small" type="button" @click="setVisibleZoom(1)" data-tip="重置为 100%"><CollabIcon name="IconZoom100" /><span>实际大小</span></button>
            </div>
          </div>
          <input ref="fileInput" type="file" accept=".pptx" style="display:none" @change="onUploadFile" />
        </div>
      </template>
    </CollabEditorRibbon>
      <div v-if="showTablePrompt" class="collab-slide-konva__modal-bg" @click="showTablePrompt = false">
        <div class="collab-slide-konva__modal" @click.stop>
          <h3>插入表格</h3>
          <label>行数 <input type="number" v-model.number="tablePrompt.rows" min="1" max="20" /></label>
          <label>列数 <input type="number" v-model.number="tablePrompt.cols" min="1" max="10" /></label>
          <div class="collab-slide-konva__modal-actions">
            <button @click="showTablePrompt = false">取消</button>
            <button @click="confirmAddTable" :disabled="!tablePrompt.rows || !tablePrompt.cols">确认</button>
          </div>
        </div>
      </div>
    <div v-if="loading" class="collab-slide-konva__loading">加载演示文稿中…</div>
    <div v-else class="collab-slide-konva__body">
      <p v-if="recoveryMessage" class="collab-slide-konva__recovery">{{ recoveryMessage }}</p>
      <aside class="collab-slide-konva__thumbs">
        <div
          v-for="(s, i) in slides"
          :key="s.raw?.path || `slide-${s.index}`"
          class="collab-slide-konva__thumb"
          :class="{ active: i === activeIndex }"
          role="button"
          tabindex="0"
          @click="activeIndex = i"
          @keydown.enter.prevent="activeIndex = i"
          @keydown.space.prevent="activeIndex = i"
        >
          <span class="collab-slide-konva__thumb-num">{{ i + 1 }}</span>
          <div class="collab-slide-konva__thumb-canvas" :data-testid="`slide-thumb-${i}`" aria-hidden="true">
            <svg class="collab-slide-konva__thumb-svg" :viewBox="thumbViewBox" preserveAspectRatio="xMidYMid meet">
              <rect :width="thumbViewport.w" :height="thumbViewport.h" :fill="s.background ? '#' + s.background : '#ffffff'" />
              <g v-for="shape in s.shapes" :key="shape.id" :transform="`translate(${emuToPx(shape.x)*thumbScale} ${emuToPx(shape.y)*thumbScale}) rotate(${shape.rotation ?? 0} ${emuToPx(shape.w)*thumbScale/2} ${emuToPx(shape.h)*thumbScale/2})`">
                <rect
                  v-if="shape.type === 'rect' || shape.type === 'roundRect'"
                  :width="emuToPx(shape.w)*thumbScale"
                  :height="emuToPx(shape.h)*thumbScale"
                  :rx="shape.type === 'roundRect' ? Math.min(emuToPx(shape.w), emuToPx(shape.h))*thumbScale*0.15 : 0"
                  :fill="shape.fill ? '#' + shape.fill : '#1f2937'"
                  :stroke="shape.stroke ? '#' + shape.stroke : 'none'"
                  :stroke-width="shape.strokeWidth ? emuToPx(shape.strokeWidth)*thumbScale : 0.6"
                />
                <ellipse
                  v-else-if="shape.type === 'ellipse'"
                  :cx="emuToPx(shape.w)*thumbScale/2"
                  :cy="emuToPx(shape.h)*thumbScale/2"
                  :rx="emuToPx(shape.w)*thumbScale/2"
                  :ry="emuToPx(shape.h)*thumbScale/2"
                  :fill="shape.fill ? '#' + shape.fill : '#10b981'"
                  :stroke="shape.stroke ? '#' + shape.stroke : 'none'"
                  :stroke-width="shape.strokeWidth ? emuToPx(shape.strokeWidth)*thumbScale : 0.6"
                />
                <line
                  v-else-if="shape.type === 'line' || shape.type === 'arrow'"
                  :x1="0" :y1="0"
                  :x2="emuToPx(shape.w)*thumbScale"
                  :y2="emuToPx(shape.h)*thumbScale"
                  :stroke="shape.stroke ? '#' + shape.stroke : '#111827'"
                  :stroke-width="shape.strokeWidth ? emuToPx(shape.strokeWidth)*thumbScale : 0.8"
                />
                <polygon
                  v-else-if="shape.type === 'triangle'"
                  :points="thumbTriangle(emuToPx(shape.w)*thumbScale, emuToPx(shape.h)*thumbScale)"
                  :fill="shape.fill ? '#' + shape.fill : '#0ea5e9'"
                />
                <polygon
                  v-else-if="shape.type === 'star'"
                  :points="thumbStar(emuToPx(shape.w)*thumbScale, emuToPx(shape.h)*thumbScale)"
                  :fill="shape.fill ? '#' + shape.fill : '#f59e0b'"
                />
                <polygon
                  v-else-if="shape.type === 'hexagon'"
                  :points="thumbHexagon(emuToPx(shape.w)*thumbScale, emuToPx(shape.h)*thumbScale)"
                  :fill="shape.fill ? '#' + shape.fill : '#22c55e'"
                />
                <rect
                  v-else-if="shape.type === 'callout'"
                  :width="emuToPx(shape.w)*thumbScale"
                  :height="Math.max(emuToPx(shape.h)*thumbScale*0.75, 6)"
                  :rx="1.5"
                  :fill="shape.fill ? '#' + shape.fill : '#fef3c7'"
                  :stroke="shape.stroke ? '#' + shape.stroke : '#92400e'"
                  :stroke-width="0.6"
                />
                <text
                  v-if="shape.text && (shape.type === 'text' || shape.type === 'rect' || shape.type === 'roundRect' || shape.type === 'ellipse' || shape.type === 'callout')"
                  :x="2" :y="Math.min(8, emuToPx(shape.h)*thumbScale*0.6)"
                  font-size="5"
                  :fill="thumbTextFill(s.background)"
                  font-family="-apple-system, BlinkMacSystemFont, sans-serif"
                >{{ shape.text.slice(0, 40) }}</text>
              </g>
            </svg>
          </div>
          <button class="collab-slide-konva__iconbtn" @click.stop="moveSlide(i, i - 1)" :disabled="i === 0" data-tip="上移">↑</button>
          <button class="collab-slide-konva__iconbtn" @click.stop="moveSlide(i, i + 1)" :disabled="i === slides.length - 1" data-tip="下移">↓</button>
          <button class="collab-slide-konva__iconbtn danger" @click.stop="deleteSlide(i)" :disabled="slides.length <= 1" data-tip="删除">×</button>
        </div>
      </aside>
      <div ref="stageWrapRef" class="collab-slide-konva__stage-wrap" data-testid="slide-stage-wrap">
        <div class="collab-slide-konva__zoom-info">{{ stageWidthPx }}×{{ stageHeightPx }} px</div>
        <v-stage
          v-if="activeSlide"
          ref="stageRef"
          :config="stageConfig"
          class="collab-slide-konva__stage"
          data-testid="slide-konva-stage"
          :aria-label="`幻灯片舞台 ${stageWidthPx}×${stageHeightPx} 像素`"
          @click="onStageClick"
          @tap="onStageClick"
        >
          <v-layer>
            <!-- background fill -->
            <v-rect
              v-if="activeSlide.background"
              :config="{ x: 0, y: 0, width: stageWidthPx, height: stageHeightPx, fill: '#' + activeSlide.background }"
            />
            <!-- v0.7.183 — master decoration layer (top bar + bottom rule + corner dots). Drawn AFTER the
                 background fill so it's not covered, BEFORE shapes so user content sits on top. -->
            <v-rect :key="'master-top-bar-' + (activeSlide?.index ?? 0)" :config="masterTopBar" />
            <v-line :config="masterBottomRule" />
            <v-circle
              v-for="(dot, idx) in masterCornerDots"
              :key="'master-dot-' + idx"
              :config="dot"
            />
            <v-rect :config="slideCornerChip" />
            <!-- shapes -->
            <template v-for="shape in activeShapes" :key="shape.id">
              <v-text
                v-if="shape.type === 'text'"
                :config="{
                  id: shape.id,
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  width: emuToPx(shape.w),
                  height: emuToPx(shape.h),
                  text: shape.text || '',
                  fontSize: shape.fontSize || 18,
                  // v0.7.137 — 字体链：先尝试 PPTX 中声明的字体 (Oranienbaum/Liter/MiSans)，
                  // 然后 fallback 到系统 CJK 字体 (PingFang SC/YaHei/MiSans)，
                  // 最后 sans-serif。没有这层 fallback，中文会显示为空白方块。
                  fontFamily: textFontFamily(shape.fontFamily),
                  // v0.7.199 — reactive fill via defaultTextFill (computed(() => ...) tracking
                  // activeSlide.background). The previous inline luminance() call was
                  // captured at v-for setup time when activeShapes was still empty,
                  // so the resulting text fill stayed at '#0f172a' even after
                  // activeSlide.value.background resolved to '1E1E1E' (luminance=0.013),
                  // making text invisible against the dark slide.
                  fill: shape.fontColor ? '#' + shape.fontColor : defaultTextFill.value,
                  draggable: true,
                  rotation: shape.rotation ?? 0,
                  name: 'shape',
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
                @dblclick="(e: any) => onTextEdit(shape.id, e)"
                @dbltap="(e: any) => onTextEdit(shape.id, e)"
              />
              <v-rect
                v-else-if="shape.type === 'rect'"
                :config="{
                  id: shape.id,
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  width: emuToPx(shape.w),
                  height: emuToPx(shape.h),
                  fill: shape.fill ? '#' + shape.fill : '#3b82f6',
                  stroke: shape.stroke ? '#' + shape.stroke : '#1e3a8a',
                  strokeWidth: 1,
                  draggable: true,
                  rotation: shape.rotation ?? 0,
                  name: 'shape',
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
              />
              <v-ellipse
                v-else-if="shape.type === 'ellipse'"
                :config="{
                  id: shape.id,
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  radiusX: emuToPx(shape.w) / 2,
                  radiusY: emuToPx(shape.h) / 2,
                  offsetX: -emuToPx(shape.w) / 2,
                  offsetY: -emuToPx(shape.h) / 2,
                  fill: shape.fill ? '#' + shape.fill : '#10b981',
                  stroke: shape.stroke ? '#' + shape.stroke : '#064e3b',
                  strokeWidth: 1,
                  draggable: true,
                  rotation: shape.rotation ?? 0,
                  name: 'shape',
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
              />
              <v-line
                v-else-if="shape.type === 'line'"
                :config="{
                  id: shape.id,
                  points: [0, 0, emuToPx(shape.w), emuToPx(shape.h)],
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  stroke: shape.stroke ? '#' + shape.stroke : '#111827',
                  strokeWidth: shape.strokeWidth ? Math.max(1, emuToPx(shape.strokeWidth)) : 2,
                  draggable: true,
                  name: 'shape',
                  rotation: shape.rotation ?? 0,
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
              />
              <v-rect
                v-else-if="shape.type === 'roundRect'"
                :config="{
                  id: shape.id,
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  width: emuToPx(shape.w),
                  height: emuToPx(shape.h),
                  cornerRadius: Math.min(emuToPx(shape.w), emuToPx(shape.h)) * 0.15,
                  fill: shape.fill ? '#' + shape.fill : '#8b5cf6',
                  stroke: shape.stroke ? '#' + shape.stroke : '#4c1d95',
                  strokeWidth: 1,
                  draggable: true,
                  name: 'shape',
                  rotation: shape.rotation ?? 0,
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
              />
              <v-path
                v-else-if="shape.type === 'arrow'"
                :config="{
                  id: shape.id,
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  data: arrowPath(emuToPx(shape.w), emuToPx(shape.h)),
                  fill: shape.fill ? '#' + shape.fill : '#ef4444',
                  stroke: shape.stroke ? '#' + shape.stroke : '#7f1d1d',
                  strokeWidth: 1,
                  draggable: true,
                  name: 'shape',
                  rotation: shape.rotation ?? 0,
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
              />
              <v-path
                v-else-if="shape.type === 'triangle'"
                :config="{
                  id: shape.id,
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  data: trianglePath(emuToPx(shape.w), emuToPx(shape.h)),
                  fill: shape.fill ? '#' + shape.fill : '#f59e0b',
                  stroke: shape.stroke ? '#' + shape.stroke : '#78350f',
                  strokeWidth: 1,
                  draggable: true,
                  name: 'shape',
                  rotation: shape.rotation ?? 0,
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
              />
              <v-path
                v-else-if="shape.type === 'star'"
                :config="{
                  id: shape.id,
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  data: starPath(emuToPx(shape.w), emuToPx(shape.h)),
                  fill: shape.fill ? '#' + shape.fill : '#fbbf24',
                  stroke: shape.stroke ? '#' + shape.stroke : '#78350f',
                  strokeWidth: 1,
                  draggable: true,
                  name: 'shape',
                  rotation: shape.rotation ?? 0,
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
              />
              <v-path
                v-else-if="shape.type === 'hexagon'"
                :config="{
                  id: shape.id,
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  data: hexagonPath(emuToPx(shape.w), emuToPx(shape.h)),
                  fill: shape.fill ? '#' + shape.fill : '#06b6d4',
                  stroke: shape.stroke ? '#' + shape.stroke : '#164e63',
                  strokeWidth: 1,
                  draggable: true,
                  name: 'shape',
                  rotation: shape.rotation ?? 0,
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
              />
              <v-path
                v-else-if="shape.type === 'callout'"
                :config="{
                  id: shape.id,
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  data: calloutPath(emuToPx(shape.w), emuToPx(shape.h)),
                  fill: shape.fill ? '#' + shape.fill : '#fef3c7',
                  stroke: shape.stroke ? '#' + shape.stroke : '#92400e',
                  strokeWidth: 1,
                  draggable: true,
                  name: 'shape',
                  rotation: shape.rotation ?? 0,
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
              />
              <v-group
                v-else-if="shape.type === 'table'"
                :config="{
                  id: shape.id,
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  draggable: true,
                  name: 'shape',
                  rotation: shape.rotation ?? 0,
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
              >
                <v-rect
                  :config="{
                    x: 0, y: 0,
                    width: emuToPx(shape.w),
                    height: emuToPx(shape.h),
                    fill: '#ffffff',
                    stroke: '#94a3b8',
                    strokeWidth: 1,
                  }"
                />
                <template v-for="(cell, ci) in (shape.cellTexts?.[0] || [])" :key="'col-' + ci">
                  <v-line
                    v-for="r in (shape.rows || 1)"
                    :key="'vl-' + ci + '-' + r"
                    :config="{
                      points: [emuToPx(shape.w) / (shape.cols || 1) * ci, 0, emuToPx(shape.w) / (shape.cols || 1) * ci, emuToPx(shape.h)],
                      stroke: '#cbd5e1',
                      strokeWidth: 1,
                      listening: false,
                    }"
                  />
</template>
                <v-line
                  v-for="ri in (shape.rows ? shape.rows - 1 : 0)"
                  :key="'hl-' + ri"
                  :config="{
                    points: [0, emuToPx(shape.h) / (shape.rows || 1) * ri, emuToPx(shape.w), emuToPx(shape.h) / (shape.rows || 1) * ri],
                    stroke: '#cbd5e1',
                    strokeWidth: 1,
                    listening: false,
                  }"
                />
                <v-text
                  v-for="(row, ri) in (shape.cellTexts || [])"
                  :key="'cell-' + ri"
                  :config="{
                    x: 4,
                    y: ri * emuToPx(shape.h) / (shape.rows || 1) + 4,
                    width: emuToPx(shape.w) / (shape.cols || 1) - 8,
                    height: emuToPx(shape.h) / (shape.rows || 1) - 8,
                    text: row.join(' | '),
                    fontSize: 11,
                    fontFamily: 'Calibri, sans-serif',
                    fill: '#1f2937',
                    listening: false,
                  }"
                />
              </v-group>
              <v-image
                v-else-if="shape.type === 'picture' && pictureImages[shape.id]"
                :config="{
                  id: shape.id,
                  x: emuToPx(shape.x),
                  y: emuToPx(shape.y),
                  width: emuToPx(shape.w),
                  height: emuToPx(shape.h),
                  image: pictureImages[shape.id],
                  draggable: true,
                  name: 'shape',
                }"
                @dragend="(e: any) => onShapeDragEnd(shape.id, e)"
                @transformend="(e: any) => onShapeTransformEnd(shape.id, e)"
                @click="(e: any) => onShapeClick(shape.id, e)"
                @tap="(e: any) => onShapeClick(shape.id, e)"
              />
            </template>
            <!-- v0.7.30 — remote peer selection outlines (per-shape) -->
            <v-rect
              v-for="rs in remoteSelections"
              :key="'rsel-' + rs.clientId + '-' + rs.shapeId"
              :config="{
                x: (remoteSelectionBounds(rs.shapeId)?.x ?? 0),
                y: (remoteSelectionBounds(rs.shapeId)?.y ?? 0),
                width: (remoteSelectionBounds(rs.shapeId)?.w ?? 0),
                height: (remoteSelectionBounds(rs.shapeId)?.h ?? 0),
                stroke: rs.color,
                strokeWidth: 2,
                dash: [8, 4],
                listening: false,
              }"
            />
            <!-- v0.7.101 — local multi-selection outlines (primary has transformer) -->
            <v-rect
              v-for="sid in multiSelectedIds"
              :key="'msel-' + sid"
              :config="{
                x: (shapeBoundsPx(sid)?.x ?? 0),
                y: (shapeBoundsPx(sid)?.y ?? 0),
                width: (shapeBoundsPx(sid)?.w ?? 0),
                height: (shapeBoundsPx(sid)?.h ?? 0),
                stroke: '#58a6ff',
                strokeWidth: 1.5,
                dash: [6, 3],
                listening: false,
              }"
            />
            <!-- v0.7.104 — group bbox: drawn only when all selected shapes share one groupId -->
            <v-rect
              v-if="groupBboxPx"
              :config="{
                x: groupBboxPx.x,
                y: groupBboxPx.y,
                width: groupBboxPx.w,
                height: groupBboxPx.h,
                stroke: '#2da44e',
                strokeWidth: 2,
                dash: [10, 4],
                listening: false,
              }"
            />
            <v-transformer
              ref="transformerRef"
              :config="{
                rotateEnabled: true,
                anchorStroke: '#58a6ff',
                borderStroke: '#58a6ff',
                anchorSize: 8,
              }"
            />
                        <!-- Remote peer cursors -->
            <v-circle
              v-for="c in remoteCursors"
              :key="c.clientId"
              :config="{
                x: emuToPx(c.x ?? 0),
                y: emuToPx(c.y ?? 0),
                radius: 6,
                fill: c.color,
                stroke: '#fff',
                strokeWidth: 2,
                listening: false,
              }"
            />
            <v-text
              v-for="c in remoteCursors"
              :key="'lbl-' + c.clientId"
              :config="{
                x: emuToPx((c.x ?? 0)) + 10,
                y: emuToPx((c.y ?? 0)) - 18,
                text: c.name,
                fontSize: 11,
                fill: c.color,
                fontStyle: 'bold',
                listening: false,
              }"
            />
          </v-layer>        </v-stage>
      </div>
      <aside class="collab-slide-konva__inspector" v-if="selectedShape">
        <h3>形状属性</h3>
        <!-- v0.7.38 Build #46.x — 飞书级格式面板: 文本 / 填充 / 描边 / 字号 / 粗斜体 / 位置 -->
        <label class="collab-slide-konva__inspector-text">
          <span>文本</span>
          <textarea v-model="inspectorText" rows="3" @change="onInspectorTextChange" />
        </label>
        <div class="collab-slide-konva__inspector-row">
          <label class="collab-slide-konva__inspector-color">
            <span>填充</span>
            <input type="color" :value="inspectorFillColor" @input="onInspectorFillPicker(($event.target as HTMLInputElement).value)" />
            <input v-model="inspectorFill" placeholder="3b82f6" class="collab-slide-konva__inspector-hex" @change="onInspectorFillChange" />
          </label>
          <label class="collab-slide-konva__inspector-color">
            <span>描边</span>
            <input type="color" :value="inspectorStrokeColor" @input="onInspectorStrokePicker(($event.target as HTMLInputElement).value)" />
            <input v-model="inspectorStroke" placeholder="1e3a8a" class="collab-slide-konva__inspector-hex" @change="onInspectorStrokeChange" />
          </label>
        </div>
        <div class="collab-slide-konva__inspector-row">
          <label class="collab-slide-konva__inspector-num">
            <span>字号</span>
            <input v-model.number="inspectorFontSize" type="number" min="6" max="200" @change="onInspectorFontSizeChange" />
          </label>
          <label class="collab-slide-konva__inspector-num">
            <span>线宽</span>
            <input v-model.number="inspectorStrokeWidth" type="number" min="0" max="20" step="0.5" @change="onInspectorStrokeWidthChange" />
          </label>
          <label class="collab-slide-konva__inspector-num">
            <span>旋转</span>
            <input v-model.number="inspectorRotation" type="number" min="0" max="359" step="1" @change="onInspectorRotationChange" data-testid="slide-inspector-rotation" />
          </label>
        </div>
        <div class="collab-slide-konva__inspector-row collab-slide-konva__inspector-toggles" v-if="selectedShape.type === 'text' || selectedShape.type === 'rect' || selectedShape.type === 'ellipse'">
          <button type="button" :class="{ active: inspectorBold }" @click="toggleBold" data-tip="粗体"><b>B</b></button>
          <button type="button" :class="{ active: inspectorItalic }" @click="toggleItalic" data-tip="斜体"><i>I</i></button>
        </div>
        <div class="collab-slide-konva__inspector-pos">
          <span>位置</span>
          <span>x {{ Math.round(selectedShape.x) }} · y {{ Math.round(selectedShape.y) }}</span>
          <span>尺寸</span>
          <span>w {{ Math.round(selectedShape.w) }} × h {{ Math.round(selectedShape.h) }}</span>
        </div>
      </aside>
    </div>
    <p v-if="error || saveError" class="collab-slide-konva__error">{{ saveError || error }}</p>
    <!-- v0.7.96 — fullscreen present mode (Teleport to body) -->
    <Teleport v-if="presentMode" to="body">
      <div class="slide-present-overlay" data-testid="slide-present-overlay" @click.self="onExitPresent">
        <div class="slide-present-shell">
          <div class="slide-present-stage">
            <svg
              :viewBox="`0 0 ${stageWidthPx} ${stageHeightPx}`"
              :width="stageWidthPx"
              :height="stageHeightPx"
              preserveAspectRatio="xMidYMid meet"
              class="slide-present-svg"
              data-testid="slide-present-svg"
            >
              <rect v-if="presentSlide?.background" x="0" y="0" :width="stageWidthPx" :height="stageHeightPx" :fill="`#${presentSlide.background}`" />
              <template v-for="shape in presentShapes" :key="shape.id">
                <rect
                  v-if="shape.type === 'rect'"
                  :x="emuToPx(shape.x)" :y="emuToPx(shape.y)"
                  :width="emuToPx(shape.w)" :height="emuToPx(shape.h)"
                  :fill="shape.fill ? `#${shape.fill}` : '#3b82f6'"
                  :stroke="shape.stroke ? `#${shape.stroke}` : '#1e3a8a'"
                  stroke-width="1"
                  :transform="shape.rotation ? `rotate(${shape.rotation} ${emuToPx(shape.x) + emuToPx(shape.w)/2} ${emuToPx(shape.y) + emuToPx(shape.h)/2})` : ''"
                />
                <rect
                  v-else-if="shape.type === 'roundRect'"
                  :x="emuToPx(shape.x)" :y="emuToPx(shape.y)"
                  :width="emuToPx(shape.w)" :height="emuToPx(shape.h)"
                  :rx="Math.min(emuToPx(shape.w), emuToPx(shape.h)) * 0.15"
                  :fill="shape.fill ? `#${shape.fill}` : '#8b5cf6'"
                  :stroke="shape.stroke ? `#${shape.stroke}` : '#4c1d95'"
                  stroke-width="1"
                  :transform="shape.rotation ? `rotate(${shape.rotation} ${emuToPx(shape.x) + emuToPx(shape.w)/2} ${emuToPx(shape.y) + emuToPx(shape.h)/2})` : ''"
                />
                <ellipse
                  v-else-if="shape.type === 'ellipse'"
                  :cx="emuToPx(shape.x) + emuToPx(shape.w)/2" :cy="emuToPx(shape.y) + emuToPx(shape.h)/2"
                  :rx="emuToPx(shape.w)/2" :ry="emuToPx(shape.h)/2"
                  :fill="shape.fill ? `#${shape.fill}` : '#10b981'"
                  :stroke="shape.stroke ? `#${shape.stroke}` : '#064e3b'"
                  stroke-width="1"
                />
                <line
                  v-else-if="shape.type === 'line'"
                  :x1="emuToPx(shape.x)" :y1="emuToPx(shape.y)"
                  :x2="emuToPx(shape.x) + emuToPx(shape.w)" :y2="emuToPx(shape.y) + emuToPx(shape.h)"
                  :stroke="shape.stroke ? `#${shape.stroke}` : '#111827'"
                  :stroke-width="shape.strokeWidth ? Math.max(1, emuToPx(shape.strokeWidth)) : 2"
                />
                <text
                  v-else-if="shape.type === 'text'"
                  :x="emuToPx(shape.x)" :y="emuToPx(shape.y) + (shape.fontSize || 18)"
                  :width="emuToPx(shape.w)"
                  :fill="shape.fill ? `#${shape.fill}` : '#1f2937'"
                  :font-size="shape.fontSize || 18"
                  font-family="Segoe UI, system-ui, sans-serif"
                >{{ shape.text || '' }}</text>
                <rect
                  v-else
                  :x="emuToPx(shape.x)" :y="emuToPx(shape.y)"
                  :width="emuToPx(shape.w)" :height="emuToPx(shape.h)"
                  :fill="shape.fill ? `#${shape.fill}` : '#94a3b8'"
                  :stroke="shape.stroke ? `#${shape.stroke}` : '#475569'"
                  stroke-width="1"
                  :transform="shape.rotation ? `rotate(${shape.rotation} ${emuToPx(shape.x) + emuToPx(shape.w)/2} ${emuToPx(shape.y) + emuToPx(shape.h)/2})` : ''"
                />
              </template>
            </svg>
          </div>
          <div class="slide-present-controls" data-testid="slide-present-controls">
            <button type="button" class="slide-present-btn" @click="presentPrev" :disabled="presentIndex === 0" data-testid="slide-present-prev" data-tip="上一页 (←)">← 上一页</button>
            <span class="slide-present-counter" data-testid="slide-present-counter">{{ presentIndex + 1 }} / {{ slides.length }}</span>
            <button type="button" class="slide-present-btn" @click="presentNext" :disabled="presentIndex >= slides.length - 1" data-testid="slide-present-next" data-tip="下一页 (→)">下一页 →</button>
            <span class="slide-present-divider" />
            <button type="button" class="slide-present-btn slide-present-btn--exit" @click="onExitPresent" data-testid="slide-present-exit" data-tip="退出 (ESC)">✕ 退出 (ESC)</button>
          </div>
          <div v-if="presentSlide?.notes" class="slide-present-notes" data-testid="slide-present-notes">
            <div class="slide-present-notes-label">演讲者备注</div>
            <div class="slide-present-notes-body">{{ presentSlide.notes }}</div>
          </div>
          <div v-if="nextSlide && nextSlide !== presentSlide" class="slide-present-next-preview" data-testid="slide-present-next-preview">
            <div class="slide-present-next-label">下一页</div>
            <div class="slide-present-next-title">{{ slideSummary(nextSlide) }}</div>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- v0.7.30 — speaker notes panel -->
    <section class="collab-slide-konva__notes" :data-collapsed="panelsCollapsed.notes ? 'true' : 'false'">
      <header class="collab-slide-konva__notes-header" @click="panelsCollapsed.notes = !panelsCollapsed.notes" :data-tip="panelsCollapsed.notes ? '展开备注' : '折叠备注'">
        <span>{{ panelsCollapsed.notes ? '▸' : '▾' }} 📝 演讲者备注</span>
        <span class="collab-slide-konva__notes-status">{{ notesStatus }}</span>
      </header>
      <textarea
        class="collab-slide-konva__notes-textarea"
        rows="4"
        :value="activeSlide?.notes ?? ''"
        :placeholder="activeSlide ? '为第 ' + (activeSlide.index + 1) + ' 页添加备注…' : '加载中…'"
        @input="onNotesInput(($event.target as HTMLTextAreaElement).value)"
        @blur="commitNotes"
      />
      <p class="collab-slide-konva__notes-hint">备注会跟随每张幻灯片一起保存到 .pptx，并在演示者视图中显示。</p>
    </section>
    <!-- v0.7.38 Build #46.x — animation timeline panel (entrance / emphasis / exit effects). -->
    <section class="collab-slide-konva__animations" :data-collapsed="panelsCollapsed.animations ? 'true' : 'false'">
      <header class="collab-slide-konva__animations-header" @click="panelsCollapsed.animations = !panelsCollapsed.animations" :data-tip="panelsCollapsed.animations ? '展开动画' : '折叠动画'">
        <span>{{ panelsCollapsed.animations ? '▸' : '▾' }} 🎬 动画 (第 {{ activeIndex + 1 }} 页)</span>
        <span class="collab-slide-konva__animations-status">{{ animations.length }} 个效果</span>
      </header>
      <!-- v0.7.64 — slide transition (inter-slide effect) -->
      <div class="collab-slide-konva__animations-toolbar">
        <label class="collab-slide-konva__animations-label">转场:
          <select v-model="transitionInput" @change="onTransitionCommit" class="collab-slide-konva__animations-select" data-tip="幻灯片切换效果">
            <option value="none">无</option>
            <option value="fade">淡入淡出</option>
            <option value="push">推出</option>
            <option value="wipe">擦除</option>
            <option value="split">分割</option>
            <option value="circle">圆形展开</option>
            <option value="cover">覆盖</option>
            <option value="pull">拉入</option>
            <option value="dissolve">溶解</option>
            <option value="zoom">缩放</option>
            <option value="morph">变形</option>
            <option value="random">随机</option>
          </select>
        </label>
      </div>
      <div class="collab-slide-konva__animations-toolbar">
        <select v-model="newEffect" class="collab-slide-konva__animations-select" data-tip="效果">
          <option value="fade">淡入</option>
          <option value="flyIn">飞入</option>
          <option value="zoom">缩放</option>
          <option value="spin">旋转</option>
          <option value="bounce">弹跳</option>
          <option value="appear">出现</option>
          <option value="disappear">消失</option>
          <option value="pulse">脉冲</option>
          <option value="colorPulse">变色脉冲</option>
          <option value="teeter">摇摆</option>
          <option value="growShrink">缩放</option>
        </select>
        <select v-model="newTrigger" class="collab-slide-konva__animations-select" data-tip="触发">
          <option value="onClick">点击时</option>
          <option value="withPrevious">与上一动画同时</option>
          <option value="afterPrevious">上一动画之后</option>
        </select>
        <button
          type="button"
          class="collab-slide-konva__animations-btn"
          :disabled="selectedId == null"
        @click="addAnimation"
        data-tip="为选中形状添加动画"
        data-testid="slide-anim-add-btn"
      >+ 添加动画</button>
      <button
        type="button"
        class="collab-slide-konva__animations-btn"
        :disabled="animations.length === 0"
        @click="clearAnimations"
        data-testid="slide-anim-clear-btn"
      >清除</button>
      </div>
      <ol v-if="animations.length" class="collab-slide-konva__animations-list">
        <li v-for="(a, idx) in animations" :key="`${a.spId}-${idx}`" class="collab-slide-konva__animations-item" :data-testid="`slide-anim-item-${idx}`">
          <span class="collab-slide-konva__animations-num">{{ idx + 1 }}</span>
          <select
            class="collab-slide-konva__animations-select"
            :value="a.effect"
            @change="onAnimationPatch(idx, 'effect', ($event.target as HTMLSelectElement).value)"
            :data-testid="`slide-anim-effect-${idx}`"
            data-tip="效果"
          >
            <option v-for="o in animEffectOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
          <select
            class="collab-slide-konva__animations-select"
            :value="a.trigger"
            @change="onAnimationPatch(idx, 'trigger', ($event.target as HTMLSelectElement).value)"
            :data-testid="`slide-anim-trigger-${idx}`"
            data-tip="触发"
          >
            <option v-for="o in animTriggerOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
          <input
            type="number"
            min="0"
            step="50"
            class="collab-slide-konva__animations-input"
            :value="a.durationMs"
            @change="onAnimationPatch(idx, 'durationMs', Number(($event.target as HTMLInputElement).value))"
            data-tip="时长 (ms)"
            :data-testid="`slide-anim-duration-${idx}`"
          />
          <input
            type="number"
            min="0"
            step="50"
            class="collab-slide-konva__animations-input"
            :value="a.delayMs"
            @change="onAnimationPatch(idx, 'delayMs', Number(($event.target as HTMLInputElement).value))"
            data-tip="延迟 (ms)"
            :data-testid="`slide-anim-delay-${idx}`"
          />
          <button
            type="button"
            class="collab-slide-konva__animations-btn"
            :disabled="idx === 0"
            @click="moveAnimation(idx, -1)"
            data-tip="上移"
            :data-testid="`slide-anim-up-${idx}`"
          >↑</button>
          <button
            type="button"
            class="collab-slide-konva__animations-btn"
            :disabled="idx === animations.length - 1"
            @click="moveAnimation(idx, 1)"
            data-tip="下移"
            :data-testid="`slide-anim-down-${idx}`"
          >↓</button>
          <button
            type="button"
            class="collab-slide-konva__animations-del"
            @click="removeAnimation(idx)"
            :data-testid="`slide-anim-del-${idx}`"
          >×</button>
        </li>
      </ol>
      <p v-else class="collab-slide-konva__animations-empty">
        选中一个形状，然后点击「+ 添加动画」为其添加入场/强调/退出效果。
      </p>
    </section>
    <!-- v0.7.113 — PPT 母版视图 modal (genoffice vendor) -->
    <div v-if="masterModalOpen" class="collab-slide-konva__modal-bg" @click.self="closeMasterModal" data-testid="slide-master-modal">
      <div class="collab-slide-konva__modal" @click.stop>
        <header class="collab-slide-konva__modal-header">
          <h3>📐 母版视图</h3>
          <span class="collab-slide-konva__modal-hint">列出当前文档的所有母版与版式，点击查看 <code>&lt;p:cSld&gt;</code> 信息，并可对 <code>&lt;p:cSld name&gt;</code> 重命名后保存。</span>
        </header>
        <div class="collab-slide-konva__modal-body">
          <ol class="collab-slide-konva__master-list" data-testid="slide-master-list">
            <li
              v-for="(p, idx) in masterParts"
              :key="p.partPath"
              :class="['collab-slide-konva__master-item', p.kind === 'master' ? 'is-master' : 'is-layout', idx === selectedMasterIdx ? 'is-active' : '']"
              :data-testid="`slide-master-item-${idx}`"
              @click="selectMasterPart(idx)"
            >
              <span class="collab-slide-konva__master-kind">{{ p.kind === 'master' ? '母版' : '版式' }}</span>
              <span class="collab-slide-konva__master-name">{{ p.name || `Part ${idx}` }}</span>
              <small class="collab-slide-konva__master-path">{{ p.partPath }}</small>
            </li>
          </ol>
          <div class="collab-slide-konva__master-detail">
            <template v-if="selectedMaster">
              <h4>{{ selectedMaster.name || 'Part ' + selectedMasterIdx }}</h4>
              <p class="collab-slide-konva__master-detail-row"><b>路径</b> <code>{{ selectedMaster.partPath }}</code></p>
              <p class="collab-slide-konva__master-detail-row"><b>种类</b> {{ selectedMaster.kind === 'master' ? '母版 (slideMaster)' : '版式 (slideLayout)' }}</p>
              <p class="collab-slide-konva__master-detail-row" data-testid="slide-master-element-count">
                <b>元素数</b>
                <span v-if="masterElementSummary">{{ masterElementSummary }}</span>
                <span v-else>（重新打开后可见）</span>
              </p>
              <div class="collab-slide-konva__master-rename">
                <label>
                  <span>cSld 名称</span>
                  <input
                    v-model="masterNameDraft"
                    :placeholder="(selectedMaster.name || '').slice(0, 40)"
                    data-testid="slide-master-name-input"
                  />
                </label>
                <button
                  type="button"
                  :disabled="!masterNameDirty"
                  data-testid="slide-master-rename-btn"
                  @click="applyMasterRename"
                >重命名</button>
              </div>
              <details class="collab-slide-konva__master-xml">
                <summary>预览 OOXML</summary>
                <pre v-if="masterPreviewXml" data-testid="slide-master-xml" style="max-height:240px;overflow:auto;">{{ masterPreviewXml }}</pre>
              </details>
              <p v-if="masterFeedback" class="collab-slide-konva__modal-error" data-testid="slide-master-feedback">{{ masterFeedback }}</p>
            </template>
            <p v-else>加载中…</p>
          </div>
        </div>
        <footer class="collab-slide-konva__modal-actions">
          <button type="button" @click="closeMasterModal" data-testid="slide-master-close-btn">关闭</button>
        </footer>
      </div>
    </div>
    <!-- v0.7.29 — comments side panel -->
    <CollabCommentsPanel
      :doc-id="props.docId"
      :token="props.token"
      :anchor="commentAnchor"
      anchor-label="当前幻灯片"
      placeholder="对当前幻灯片或所选形状添加评论…"
    />

    <!-- v0.7.74 — PPT bottom status bar (GenOffice style: slide count / theme / zoom) -->
    <div class="collab-slide-konva__statusbar" v-if="!presentMode">
      <span class="collab-slide-konva__statusbar-item">{{ activeIndex + 1 }} / {{ slides.length }} 张幻灯片</span>
      <span class="collab-slide-konva__statusbar-sep">·</span>
      <span class="collab-slide-konva__statusbar-item">{{ themeMeta?.name || 'Office' }} 主题</span>
      <span class="collab-slide-konva__statusbar-sep">·</span>
      <span class="collab-slide-konva__statusbar-item">{{ connectionLabel }}</span>
      <span class="collab-slide-konva__statusbar-spacer"></span>
      <span class="collab-slide-konva__statusbar-item">{{ selectedId ? '已选中' : '无选中' }}</span>
      <span class="collab-slide-konva__statusbar-sep">·</span>
      <button class="collab-slide-konva__statusbar-btn" type="button" data-testid="slide-zoom-out" @click="onZoomOut">−</button>
      <span class="collab-slide-konva__statusbar-zoom">{{ slideZoomPercent }}%</span>
      <button class="collab-slide-konva__statusbar-btn" type="button" data-testid="slide-zoom-in" @click="onZoomIn">＋</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch, onMounted } from 'vue'
import { useTheme } from '@/composables/useTheme'
import * as Y from 'yjs'
import { useYjsCollabDoc } from '@/composables/useYjsCollabDoc'
import {
  openPptxShapes,
  newPptxShapeDeck,
  insertBlankSlideOnDeck,
  addShapeOnDeck,
  savePptxShapeBytes,
  addTableToSlide,
  setSlideNotesOnDeck,
  emuToPx,
  getSlideAnimationsOnDeck,
  setSlideAnimationsOnDeck,
  patchSlideAnimationOnDeck,
  reorderSlideAnimationOnDeck,
  getShapeSpIdOnDeck,
  getSlideTransitionOnDeck,
  setSlideTransitionOnDeck,
  setSlideLayout,
  resetSlideLayout,
  listSlideLayouts,
  ensureBuiltinLayout,
  listMasterPartsOnDeck,
  parseMasterToSlideOnDeck,
  readMasterPartXmlOnDeck,
  writeMasterPartXmlOnDeck,
  renameMasterOnDeck,
  applyThemeToDeck,
  recolorDeck,
  type PptxShape,
  type PptxShapeSlide,
  type PptxShapeDeck,
  type SlideAnimationRecord,
  type AnimEffectKind,
  type AnimTrigger,
} from '@/editor/adapters/pptxShapeAdapter'
import type { SlideTransitionKind } from '@/editor/engines/pptx-engine/generate'
import type { Slide } from '@/editor/engines/pptx-engine/types'
import type { MasterPartInfo as EngineMasterPartInfo } from '@/editor/engines/pptx-engine/index'
// v0.7.107 — wire the engine's groupElements / ungroupElement into the save path
// so a local grouping operation writes a real <p:grpSp> to ppt/slides/slideN.xml.
import { groupElements, ungroupElement } from '@/editor/engines/pptx-engine/index'
import {
  projectGroupsToEngine,
  markLocalGrouped,
  markLocalUngrouped,
} from '../../editor/adapters/slideGroupSync'
import type { SlideThemePreset } from '@/editor/slides/themes/genofficeThemes'
import { addSlideComment, getSlideComments } from '@/editor/engines/pptx-engine/comments'
import type { CollabDocComment } from '@/api/collabDoc'
import {
  downloadCollabDocBytes,
  uploadCollabDocBytes,
} from '@/api/collabDoc'
import { MessagePlugin } from 'tdesign-vue-next'
import { stepRotation90, normalizeRotation, formatRotation } from '@/editor/adapters/slideRotation'
import CollabCommentsPanel from '@/components/collab/CollabCommentsPanel.vue'
import CollabSlideThemePanel from '@/components/collab/CollabSlideThemePanel.vue'
import CollabEditorRibbon from '@/components/collab/CollabEditorRibbon.vue'
import CollabIcon from '@/components/collab/CollabIcon.vue'


const props = defineProps<{
  docId: string
  title: string
  token: string
  displayName: string
  /** Tenant id forwarded to the IndexedDB persistence layer. */
  tenantId?: number | string
}>()

const kindLabel = '演示文稿'
const loading = ref(true)
const error = ref<string | null>(null)
const recoveryMessage = ref<string | null>(null)
const saveError = ref<string | null>(null)
const uploading = ref(false)
const downloading = ref(false)
const saveLabel = ref('未修改')
const presentMode = ref(false)
const savetagClass = reactive({ dirty: false, saving: false })
const fileInput = ref<HTMLInputElement | null>(null)
const activeIndex = ref(0)
const slides = ref<PptxShapeSlide[]>([])
const deck = ref<PptxShapeDeck | null>(null)
const pictureImages = reactive<Record<string, HTMLImageElement>>({})
const selectedId = ref<string | null>(null)
// v0.7.101 — multi-select set (primary = last clicked, kept in selectedId).
const selectedIds = ref<string[]>([])
// v0.7.107 — yjs groupId -> engine element id map. The engine's groupElements
// returns its own internal id for the new <p:grpSp>; we stash it so the next
// ungroup call can pass that id (not the Yjs groupId) to engineUngroupElement.
const engineGroupIdByYjsGroupId = reactive<Record<string, string>>({})
// v0.7.108 — track which Yjs groupIds we have already projected onto the
// engine side per slide. Used by syncFromY to detect new (grouped on a remote
// peer) and disappeared (ungrouped on a remote peer) groups and apply
// groupElements / ungroupElement accordingly.
const lastSyncedYjsGroupIdsBySlideIdx = new Map<number, Set<string>>()

// --- Table insertion prompt ---
const showTablePrompt = ref(false)
const tablePrompt = ref({ rows: 3, cols: 3 })
const promptAddTable = () => { showTablePrompt.value = true; tablePrompt.value = { rows: 3, cols: 3 } }
const confirmAddTable = () => {
  if (!ydeck || !deck.value?.opened) return
  const { rows, cols } = tablePrompt.value
  if (!rows || !cols) return
  const cellW = 914400 * 1.4
  const cellH = 457200 * 1.0
  const offset = { x: 914400, y: 914400, w: cellW * cols, h: cellH * rows }
  const newTable = addTableToSlide(deck.value as unknown as PptxShapeDeck, activeIndex.value, rows, cols, offset)
  if (!newTable) { showTablePrompt.value = false; return }
  ydeck.doc?.transact(() => {
    let yslide = ydeck!.get(activeIndex.value) as Y.Map<unknown> | undefined
    if (!yslide) return
    let yshapes = yslide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    if (!yshapes) {
      yshapes = new Y.Array<Y.Map<unknown>>()
      yslide.set('shapes', yshapes)
    }
    const m = new Y.Map<unknown>()
    m.set('id', newTable.id)
    m.set('type', 'table')
    m.set('x', newTable.x); m.set('y', newTable.y); m.set('w', newTable.w); m.set('h', newTable.h)
    m.set('rows', rows); m.set('cols', cols)
    m.set('cellTexts', JSON.stringify(newTable.cellTexts ?? []))
    m.set('spIndex', newTable.spIndex ?? -1)
    m.set('sourceType', 'graphicFrame')
    m.set('preset', 'table')
    yshapes.push([m])
    selectOnly(newTable.id)
    scheduleSave()
  })
  showTablePrompt.value = false
}

// --- Shape path generators (for v-path presets) ---
const arrowPath = (w: number, h: number) => {
  const head = Math.min(w * 0.3, 32)
  return `M0 ${h / 2} L${w - head} ${h / 2} L${w - head} 0 L${w} ${h / 2} L${w - head} ${h} L${w - head} ${h / 2} Z`
}
const trianglePath = (w: number, h: number) => `M${w / 2} 0 L${w} ${h} L0 ${h} Z`
const starPath = (w: number, h: number) => {
  const cx = w / 2, cy = h / 2
  const rx = w / 2, ry = h / 2
  const points: Array<[number, number]> = []
  for (let i = 0; i < 10; i++) {
    const r = i % 2 === 0 ? 1 : 0.45
    const angle = (Math.PI * 2 * i) / 10 - Math.PI / 2
    points.push([cx + rx * r * Math.cos(angle), cy + ry * r * Math.sin(angle)])
  }
  return points.reduce((acc, [x, y], i) => acc + (i === 0 ? `M${x} ${y}` : ` L${x} ${y}`), '') + ' Z'
}
const hexagonPath = (w: number, h: number) => {
  const cx = w / 2, cy = h / 2
  const rx = w / 2, ry = h / 2
  const pts: Array<[number, number]> = []
  for (let i = 0; i < 6; i++) {
    const angle = (Math.PI * 2 * i) / 6 - Math.PI / 2
    pts.push([cx + rx * Math.cos(angle), cy + ry * Math.sin(angle)])
  }
  return pts.reduce((acc, [x, y], i) => acc + (i === 0 ? `M${x} ${y}` : ` L${x} ${y}`), '') + ' Z'
}
const calloutPath = (w: number, h: number) => {
  // Rounded rectangle body + downward-pointing tail (left-third).
  const r = Math.min(w, h) * 0.12
  const tailW = Math.min(w * 0.18, 28)
  const tailH = Math.min(h * 0.22, 28)
  return (
    `M${r} 0 L${w - r} 0 Q${w} 0 ${w} ${r} L${w} ${h - r} Q${w} ${h} ${w - r} ${h} L${w * 0.35 + tailW} ${h} L${w * 0.35 + tailW / 2} ${h + tailH} L${w * 0.35} ${h} L${r} ${h} Q0 ${h} 0 ${h - r} L0 ${r} Q0 0 ${r} 0 Z`
  )
}

// --- Awareness (remote cursors + presence) ---
const remoteCursors = ref<Array<{ clientId: number; x?: number; y?: number; color: string; name: string }>>([])
/** v0.7.30 — remote shape selections (for outline rendering). */
const remoteSelections = ref<Array<{ clientId: number; shapeId: string; color: string; name: string }>>([])
/** Look up a shape's display bounds in CSS pixels; returns null when the shape
 *  isn't on the active slide (e.g. peer switched slides). */
const remoteSelectionBounds = (shapeId: string): { x: number; y: number; w: number; h: number } | null => {
  const shape = activeShapes.value.find((s) => s.id === shapeId)
  if (!shape) return null
  return {
    x: emuToPx(shape.x),
    y: emuToPx(shape.y),
    w: emuToPx(shape.w),
    h: emuToPx(shape.h),
  }
}
/** v0.7.101 — local multi-selection: shapes selected besides the primary. */
const multiSelectedIds = computed(() => selectedIds.value.filter((id) => id !== selectedId.value))

/** v0.7.104 — bounding box for the current group (only when all selected shapes share a groupId). */
const groupBboxPx = computed(() => {
  const shapes = selectedShapes.value
  if (!shapes.length) return null
  const gid = selectedShapeGroupId.value
  if (!gid) return null
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
  for (const s of shapes) {
    minX = Math.min(minX, s.x); minY = Math.min(minY, s.y)
    maxX = Math.max(maxX, s.x + s.w); maxY = Math.max(maxY, s.y + s.h)
  }
  return {
    x: emuToPx(minX),
    y: emuToPx(minY),
    w: emuToPx(maxX - minX),
    h: emuToPx(maxY - minY),
  }
})
const shapeBoundsPx = (shapeId: string): { x: number; y: number; w: number; h: number } | null => {
  const shape = activeShapes.value.find((s) => s.id === shapeId)
  if (!shape) return null
  return {
    x: emuToPx(shape.x),
    y: emuToPx(shape.y),
    w: emuToPx(shape.w),
    h: emuToPx(shape.h),
  }
}

// --- Speaker notes ---
const notesStatus = ref('未修改')
const notesTimer = ref<number | null>(null)
const onNotesInput = (text: string) => {
  if (!activeSlide.value) return
  // Locally update the slide's notes (UI only) — persist to engine + schedule save.
  activeSlide.value.notes = text
  notesStatus.value = '编辑中…'
  if (notesTimer.value) window.clearTimeout(notesTimer.value)
  notesTimer.value = window.setTimeout(() => commitNotes(), 800)
}
const commitNotes = () => {
  if (!deck.value || !activeSlide.value) return
  const ok = setSlideNotesOnDeck(deck.value, activeIndex.value, activeSlide.value.notes ?? '')
  notesStatus.value = ok ? '已同步' : '保存失败'
  if (ok) scheduleSave()
}

// --- Undo/redo (Yjs undoManager) ---
const canUndo = ref(false)
const canRedo = ref(false)
const onUndo = () => { try { undoManagerRef.value?.undo?.() } catch {} }
const onRedo = () => { try { undoManagerRef.value?.redo?.() } catch {} }
const undoManagerRef = ref<any>(null)

// --- Keyboard shortcuts (Ctrl/Cmd+Z, Shift+Ctrl/Cmd+Z) ---
const onKeydown = (e: KeyboardEvent) => {
  const meta = e.ctrlKey || e.metaKey
  if (meta && e.key.toLowerCase() === 'z') {
    e.preventDefault()
    if (e.shiftKey) onRedo()
    else onUndo()
  }
}

// Yjs handles
let ydoc: Y.Doc | null = null
let ydeck: Y.Array<Y.Map<unknown>> | null = null
const connected = ref(false)
const peers = ref<Array<{ clientId: number; displayName: string; color: string }>>([])
let handle: ReturnType<typeof useYjsCollabDoc> | null = null

const PX_PER_INCH = 96
const SLIDE_W_INCH = 10 // 16:9 at 10in wide
const SLIDE_H_INCH = (10 * 9) / 16

const stageWidthPx = computed(() => Math.round(emuToPx(deck.value?.slides[0]?.width ?? SLIDE_W_INCH * 914400)))
const stageHeightPx = computed(() => Math.round(emuToPx(deck.value?.slides[0]?.height ?? SLIDE_H_INCH * 914400)))

const stageRef = ref<any>(null)
const stageWrapRef = ref<HTMLElement | null>(null)

// v0.7.100 — Konva stage 物理尺寸 = wrap 容器可用宽度 × slide 原始比例
// v-stage 的 :config 在 mount 时读一次，之后不再响应 computed 变化。
// 所以我们用 stageRef.value.getStage().size() 主动同步 Konva stage。
const stageLogicalW = ref(0)
const stageLogicalH = ref(0)
// v0.7.132 — Konva 9 Stage 构造时 bufferCanvas 不接收 pixelRatio，
// 导致 retina 屏 canvas 内部缓冲停留在 CSS 尺寸，文字发糊。
// 必须显式调用 layer.canvas.setPixelRatio(dpr) 才能生效（参考 genoffice SlideCanvas.tsx:572）。
const stageConfig = computed(() => ({
  width: stageWidthPx.value,
  height: stageHeightPx.value,
}))
// v0.7.132 — HiDPI: 把 devicePixelRatio 显式应用到所有内部 canvas
// (bufferCanvas + bufferHitCanvas + 每个 layer 的 canvas/hitCanvas)。
// Konva 9 的 Stage.size() 触发的 _resizeDOM 不会传播 pixelRatio，
// 所以每次 size 之后都要重新设置一次。
const applyKonvaHiDPI = (stage: any) => {
  if (!stage) return
  const dpr = typeof window !== 'undefined' ? (window.devicePixelRatio || 1) : 1
  // v0.7.148 — 不要把 DPR 钉死在 2，retina/3K 屏 (DPR=3) 会强制降回 2，导致
  // 文字发糊、PPT 看起来"空白"。跟随实际 devicePixelRatio（mac 上常见 2 或 3）。
  const ratio = Math.max(1, dpr)
  const apply = (c: any) => {
    // v0.7.151 — 移除 `pixelRatio !== ratio` 早退检查。Konva 9 在 Stage.setSize /
    // setAttrs 重分配 canvas 时会把 pixelRatio 重置回默认值；如果 ratio 恰好
    // 等于 Konva.pixelRatio（常见的 1x/2x 屏），apply 早退会导致 layer canvas
    // 永远停在 DPR=1，retina 屏文字被浏览器拉伸后糊成"空白"。现在无条件
    // 调用 setPixelRatio，setPixelRatio 内部 setSize 会重算 backing store。
    if (c && typeof c.setPixelRatio === 'function') {
      c.setPixelRatio(ratio)
    }
  }
  apply(stage.bufferCanvas)
  apply(stage.bufferHitCanvas)
  if (typeof stage.getLayers === 'function') {
    stage.getLayers().forEach((l: any) => {
      apply(l.canvas)
      apply(l.hitCanvas)
    })
  } else if (Array.isArray(stage.children)) {
    stage.children.forEach((l: any) => {
      apply(l.canvas)
      apply(l.hitCanvas)
    })
  }
  if (typeof stage.batchDraw === 'function') stage.batchDraw()
}

// v0.7.151 — 一次性把"应用 DPR + 重画全部 layer"打包。Konva 9 + vue-konva 3.4
// 有几个常见坑：(a) v-layer 的 onMounted 在 v-stage 之前跑，但 stageRef
// watch 的 nextTick 不一定拿到 layer.canvas；(b) Konva setSize 会重置
// canvas pixelRatio；(c) v-for shapes 异步追加不会自动触发 layer batchDraw。
// 这里提供一个不依赖任何 ref 的 kickRedraw，从外部 DOM 兜底取 stage。
const kickRedraw = () => {
  const wrap = stageWrapRef.value
  const stage = stageRef.value?.getStage?.() || (window as any).__wkStage
  if (!stage || !wrap) return
  applyKonvaHiDPI(stage)
  const layers = typeof stage.getLayers === 'function'
    ? stage.getLayers()
    : (Array.isArray(stage.children) ? stage.children : [])
  for (const l of layers) {
    if (l && typeof l.batchDraw === 'function') l.batchDraw()
    if (l && typeof l.draw === 'function') l.draw()
  }
  if (typeof stage.batchDraw === 'function') stage.batchDraw()
  if (typeof stage.draw === 'function') stage.draw()
}

const fitStage = () => {
  if (typeof window === 'undefined') return
  const wrap = stageWrapRef.value
  if (!wrap) return
  const logicalW = Math.max(1, stageWidthPx.value)
  const logicalH = Math.max(1, stageHeightPx.value)
  const styles = window.getComputedStyle(wrap)
  const horizontalPadding = parseFloat(styles.paddingLeft) + parseFloat(styles.paddingRight)
  const verticalPadding = parseFloat(styles.paddingTop) + parseFloat(styles.paddingBottom)
  const infoHeight = wrap.querySelector('.collab-slide-konva__zoom-info')?.getBoundingClientRect().height ?? 0
  // v0.7.131 — 留 8px 安全边距，避免阴影/边框被裁剪
  const maxW = Math.max(320, wrap.clientWidth - horizontalPadding - 8)
  const maxH = Math.max(180, wrap.clientHeight - verticalPadding - infoHeight - 16)
  const ratio = logicalW / logicalH
  let w = maxW
  let h = w / ratio
  if (h > maxH) { h = maxH; w = h * ratio }
  w = Math.max(1, Math.floor(w))
  h = Math.max(1, Math.floor(h))
  stageLogicalW.value = w
  stageLogicalH.value = h
  const candidate = stageRef.value
  const k = candidate?.getStage?.() || candidate || (window as any).__wkStage
  const stageEl = wrap.querySelector('.collab-slide-konva__stage, [data-testid="slide-konva-stage"]') as HTMLElement | null
  const content = wrap.querySelector('.konvajs-content') as HTMLElement | null
  const scaleX = w / logicalW
  const scaleY = h / logicalH
  if (k && typeof k.size === 'function') {
    // Konva must own the responsive scale. Scaling only `.konvajs-content`
    // leaves the internal canvas at its logical size and can produce a blank
    // canvas when the stage is mounted before the PPTX finishes loading.
    k.size({ width: w, height: h })
    if (typeof k.scale === 'function') k.scale({ x: scaleX, y: scaleY })
    applyKonvaHiDPI(k)
    if (typeof k.draw === 'function') k.draw()
  }
  if (stageEl) {
    // The Konva container must use the rendered size. Keeping it at the
    // logical 1280×720 size makes flex/max-size CSS clip the canvas before
    // the child transform has a chance to scale it into the available area.
    stageEl.style.width = `${w}px`
    stageEl.style.height = `${h}px`
    stageEl.style.maxWidth = 'none'
    stageEl.style.maxHeight = 'none'
    stageEl.style.overflow = 'hidden'
  }
  if (content) {
    content.style.width = `${w}px`
    content.style.height = `${h}px`
    content.style.transform = 'none'
  }
}

// v0.7.100 — fit-to-container: stageWrap 可用空间内按比例缩放 stage
// stageScale moved below to avoid TDZ on slideZoom

const localSlidesByIndex = computed(() => slides.value)
const activeSlide = computed(() => slides.value[activeIndex.value] ?? null)
const activeShapes = computed(() => activeSlide.value?.shapes ?? [])
const selectedShape = computed(() => activeShapes.value.find((s) => s.id === selectedId.value) ?? null)

const connectionLabel = computed(() => (connected.value ? '在线' : '离线'))
const themeMeta = computed(() => ({ name: 'Office' }))
const initialOf = (s: string) => (s || '?').trim().charAt(0).toUpperCase()

const slideSummary = (s: PptxShapeSlide) => {
  const t = s.shapes.find((x) => x.type === 'text' && x.text)
  if (t && t.text) return t.text.split('\n')[0].slice(0, 30)
  return `幻灯片 ${s.index + 1}`
}

// --- Ribbon tabs (genoffice-style tab switching) ---
type RibbonTabId = 'home' | 'insert' | 'draw' | 'design' | 'transitions' | 'animate' | 'slideshow' | 'review' | 'view'
const panelsCollapsed = reactive({ notes: true, animations: true, comments: true })

// v0.7.119 — Home-tab popovers: layout picker (from-split), layout switch (apply),
// and slide-show mode (从开始 / 从当前页). Shared click-outside closes them.
const layoutOpen = ref(false)
// v0.7.145 — collapse state for secondary groups (GenOffice pattern).
// When narrow, the insertion / play groups collapse to a single button +
// dropdown to reduce horizontal density.
// v0.7.149 — 默认展开所有 group（之前折叠以省空间，现在学习 GenOffice 让 ribbon 更饱满）.
const insertCollapsed = ref(false)
const playCollapsed = ref(false)
const arrangeCollapsed = ref(false)
// v0.7.190 — GenOffice spec: slides group ships collapsed by default;
// clicking the big button reveals the new-slide split + layout dropdown.
const slidesCollapsed = ref(true)
const slidesPanelOpen = ref(false)
const toggleInsertCollapse = () => {
  insertCollapsed.value = !insertCollapsed.value
}
const togglePlayCollapse = () => {
  playCollapsed.value = !playCollapsed.value
}
const toggleArrangeCollapse = () => {
  arrangeCollapsed.value = !arrangeCollapsed.value
}
const toggleSlidesPanel = () => {
  slidesPanelOpen.value = !slidesPanelOpen.value
  if (slidesPanelOpen.value) {
    layoutOpen.value = false
    layoutPickOpen.value = false
    slideShowOpen.value = false
  }
}
const layoutPickOpen = ref(false)
const slideShowOpen = ref(false)
/* v0.7.131 — 设计 tab 「布局」按钮的 dropdown 开合状态。
 * 模板 line 213-246 引用了 layoutMenuOpen 但之前从未 define ref，所以
 * Vue 一直 warn「Property layoutMenuOpen was accessed during render but is not defined」。
 * 这里补上。 */
const layoutMenuOpen = ref(false)
const slideShowFromStart = ref(true)
const toggleLayoutPicker = () => {
  layoutOpen.value = !layoutOpen.value
  layoutPickOpen.value = false
  slideShowOpen.value = false
}
const toggleLayoutPick = () => {
  layoutPickOpen.value = !layoutPickOpen.value
  layoutOpen.value = false
  slideShowOpen.value = false
}
const closeHomePopovers = (ev: MouseEvent) => {
  const t = ev.target as HTMLElement | null
  if (!t) return
  if (t.closest('.rb-drop-wrap')) return
  layoutOpen.value = false
  layoutPickOpen.value = false
  slideShowOpen.value = false
  slidesPanelOpen.value = false
}
onMounted(() => {
  if (typeof window !== 'undefined') document.addEventListener('click', closeHomePopovers)
})
onBeforeUnmount(() => {
  if (typeof window !== 'undefined') document.removeEventListener('click', closeHomePopovers)
})

// Pick a layout AND add a new slide in one shot (split-button path).
const applyLayoutFromSplit = (v: string) => {
  layoutOpen.value = false
  // First add a blank slide (mirrors addSlide), then apply the chosen layout.
  addSlide()
  applyLayout(v)
}
const onPresentSplitMain = () => {
  slideShowOpen.value = false
  if (slideShowFromStart.value) onEnterPresentFromStart()
  else onEnterPresent()
}
const onAddSection = () => {
  // v0.7.119 — Section creation is not yet wired into the deck model; surface
  // as a placeholder toast so users see the button is wired and not dead.
  MessagePlugin.info('节 (Section) 占位：当前 pptx-engine 未持久化节，后续接入 sections 编辑器')
}

const applyLayout = (v: string) => {
  if (!v || !deck.value) return
  let layoutPath = v
  if (v.startsWith('builtin:')) {
    const key = v.slice('builtin:'.length)
    const inserted = ensureBuiltinLayout(
      deck.value as unknown as PptxShapeDeck,
      sizeW.value,
      sizeH.value,
      key,
    )
    if (!inserted) {
      MessagePlugin.error(`内置布局 ${key} 注入失败`)
      return
    }
    layoutPath = inserted
  }
  const ok = setSlideLayout(
    deck.value as unknown as PptxShapeDeck,
    activeIndex.value,
    layoutPath,
  )
  if (ok) {
    savetagClass.dirty = true
    saveLabel.value = '布局已切换 · 待保存'
    scheduleSave()
    MessagePlugin.success(`已切换到布局 ${layoutPath}`)
  } else {
    MessagePlugin.error('布局切换失败')
  }
}

const ribbonTabs: { id: RibbonTabId; label: string }[] = [
  { id: 'home', label: '开始' },
  { id: 'insert', label: '插入' },
  { id: 'draw', label: '绘图' },
  { id: 'design', label: '设计' },
  { id: 'transitions', label: '切换' },
  { id: 'animate', label: '动画' },
  { id: 'slideshow', label: '幻灯片放映' },
  { id: 'review', label: '审阅' },
  { id: 'view', label: '视图' },
]
const activeTab = ref<RibbonTabId>('home')
const drawingTools = computed(() => shapeTools.slice(0, 3))

const shapeTools: Array<{ type: Exclude<PptxShape['type'], 'table' | 'picture'>; label: string; icon: string; testId?: string }> = [
  { type: 'roundRect', label: '圆角矩形', icon: 'IconRoundRect' },
  { type: 'ellipse', label: '椭圆', icon: 'IconEllipse', testId: 'slide-add-ellipse' },
  { type: 'line', label: '直线', icon: 'IconLine', testId: 'slide-add-line' },
  { type: 'arrow', label: '箭头', icon: 'IconArrow' },
  { type: 'triangle', label: '三角形', icon: 'IconTriangle' },
  { type: 'star', label: '星形', icon: 'IconStar' },
  { type: 'hexagon', label: '六边形', icon: 'IconHexagon' },
  { type: 'callout', label: '标注', icon: 'IconCallout' },
]
// v0.7.198 — chrome 始终跟全局 theme-mode 走, 不再随 slide 内容 luminance 反转。
//   修复「全局 light + slide bg dark → chrome 被拉成 dark 跟全局反」的问题。
//   slide 内容是 dark 还是 light 是 slide 自己的事, 编辑器 chrome 永远跟全局。
const { currentTheme: globalThemeMode } = useTheme()
const ribbonTheme = ref<'light' | 'dark'>('light')
const resolveGlobalRbTheme = (): 'light' | 'dark' => {
  const t = globalThemeMode.value
  if (t === 'light' || t === 'dark') return t
  if (typeof window !== 'undefined' && window.matchMedia)
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  return 'light'
}
const luminance = (hex: string | undefined): number => {
  if (!hex) return 1
  let h = hex.trim().replace(/^#/, '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  if (!/^[0-9a-fA-F]{6}$/.test(h)) return 1
  const r = parseInt(h.slice(0, 2), 16)
  const g = parseInt(h.slice(2, 4), 16)
  const b = parseInt(h.slice(4, 6), 16)
  // WCAG relative luminance
  const toLin = (c: number) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
  }
  return 0.2126 * toLin(r) + 0.7152 * toLin(g) + 0.0722 * toLin(b)
}
// v0.7.198 — 简化: chrome 始终跟全局 theme-mode. 旧 luminance 逻辑会跟全局反, 弃用。
watch(
  () => globalThemeMode.value,
  () => {
    ribbonTheme.value = resolveGlobalRbTheme()
  },
  { immediate: true },
)
const slideZoom = ref(1)
// v0.7.118 — visible 比例 = fit 容器缩放 × 用户 zoom 倍率。默认 slideZoom=1
// 表示「按容器自适应大小」；点「实际大小」会把 visible 设到 100%。

// v0.7.183 — master decoration layer (visual density; addresses "PPT看起来空白" complaint)
// Renders a thin top accent bar + bottom hairline rule + 3x3 corner dots so that
// even a Title-Slide-only deck reads as "designed chrome" instead of "white board".
// Tuned to GenOffice master slide accent colors — slate-50 backdrop, slate-200 rule,
// accent blue top bar (matches the brand color used in the ribbon chrome).
const MASTER_ACCENT_BLUE = '#1f6feb'
const MASTER_RULE_GRAY    = '#cbd5e1'
const MASTER_BACKDROP_TINT = '#f8fafc'
const masterBackdrop = computed(() => null as any) // v0.7.184 — 不再使用，模板里背景由 <v-rect v-if="activeSlide.background"> 提供
const masterTopBar = computed(() => ({
  x: 0, y: 0,
  width: stageWidthPx.value,
  height: 4,
  fill: MASTER_ACCENT_BLUE,
  listening: false,
}))
const masterBottomRule = computed(() => ({
  // 1px hairline along the bottom of the slide (matches GenOffice slide rule)
  points: [0, stageHeightPx.value - 1, stageWidthPx.value, stageHeightPx.value - 1],
  stroke: MASTER_RULE_GRAY,
  strokeWidth: 1,
  listening: false,
}))
// 4 small dots in the bottom-right corner — GenOffice-style page indicator marks
// placed low-profile so they never overlap with content placeholders.
const masterCornerDots = computed(() => {
  const baseX = stageWidthPx.value - 28
  const baseY = stageHeightPx.value - 22
  return [0, 1, 2].map((row) =>
    [0, 1, 2].map((col) => ({
      x: baseX + col * 8,
      y: baseY + row * 8,
      radius: 1.5,
      fill: '#94a3b8',
      listening: false,
    })),
  ).flat()
})
const slideCornerChip = computed(() => ({
  // PowerPoint-style "this is slide N" small mark; doesn't collide with content
  x: stageWidthPx.value - 16,
  y: stageHeightPx.value - 8,
  width: 6, height: 6,
  fill: MASTER_ACCENT_BLUE,
  listening: false,
}))

const fitScale = computed(() =>
  stageLogicalW.value > 0 && stageWidthPx.value > 0
    ? stageLogicalW.value / stageWidthPx.value
    : 1,
)
const visibleScale = computed(() => fitScale.value * slideZoom.value)
const slideZoomPercent = computed(() => Math.round(visibleScale.value * 100))
const setVisibleZoom = (target: number) => {
  const fs = fitScale.value || 1
  slideZoom.value = +Math.max(0.25, Math.min(4, target / fs)).toFixed(3)
  applySlideZoom()
}
const onZoomIn = () => setVisibleZoom(visibleScale.value + 0.1)
const onZoomOut = () => setVisibleZoom(visibleScale.value - 0.1)
const applySlideZoom = () => {
  // v0.7.131 — 用 Konva 内置 stage.scale() + size() 替代 CSS zoom：
  // CSS zoom 只放大显示，canvas 内部缓冲不变，文字会糊。
  // Konva 重新设 size 会按 pixelRatio 重画底层 bitmap，缩到高 zoom 也保持锐利。
  const fs = fitScale.value || 1
  const visibleScale = fs * slideZoom.value
  const logicalW = stageWidthPx.value
  const logicalH = stageHeightPx.value
  // stage 容器 = 可见缩放后大小（fit × 用户 zoom），不再是固定 fit。
  // Konva 内部 buffer = logical × visibleScale（高 zoom = 更多像素 = 锐利文字）。
  const cssW = Math.max(1, Math.floor(logicalW * visibleScale))
  const cssH = Math.max(1, Math.floor(logicalH * visibleScale))
  const bufferW = cssW
  const bufferH = cssH
  const candidate = stageRef.value
  const k = (candidate && (candidate as any).getStage?.()) || (window as any).__wkStage
  const stageEl = document.querySelector('.collab-slide-konva__stage') as HTMLElement | null
  const content = document.querySelector('.konvajs-content') as HTMLElement | null
  if (k && typeof k.size === 'function') {
    k.size({ width: bufferW, height: bufferH })
    if (typeof k.scale === 'function') {
      // 让 logical (1280x720) 的形状按 visibleScale 渲染到 buffer
      k.scale({ x: visibleScale, y: visibleScale })
    }
    applyKonvaHiDPI(k)
    if (typeof k.draw === 'function') k.draw()
  }
  if (stageEl) {
    stageEl.style.width = `${cssW}px`
    stageEl.style.height = `${cssH}px`
    stageEl.style.maxWidth = 'none'
    stageEl.style.maxHeight = 'none'
  }
  if (content) {
    content.style.width = `${cssW}px`
    content.style.height = `${cssH}px`
    content.style.transform = 'none'
  }
  // v0.7.151 — zoom 改动后强制应用 DPR + 重画，避免 setSize 把 layer canvas
  // 重置回 pixelRatio=1 导致 retina 屏文字发糊。
  kickRedraw()
}

// stage 尺寸跟随 wrap：mount 后 + ResizeObserver 触发主动 fit
let _stageFitObserver: ResizeObserver | null = null
onMounted(() => {
  nextTick(() => {
    fitStage()
    kickRedraw()
    // 再延迟一帧确保 wrap 真实尺寸
    requestAnimationFrame(() => {
      fitStage()
      kickRedraw()
    })
    // v0.7.151 — Konva 9 + vue-konva 3.4 在 v-layer mounted + stage buffer 重分配后
    // 才会把 layer.canvas 加入 stage.getLayers()。前面 nextTick + raf 拿到的
    // 可能是空 layers。这里再多一帧 + 50ms 兜底重画，覆盖所有时序组合。
    requestAnimationFrame(() => requestAnimationFrame(() => kickRedraw()))
    setTimeout(() => kickRedraw(), 50)
  })
  if (typeof ResizeObserver !== 'undefined') {
    _stageFitObserver = new ResizeObserver(() => fitStage())
    if (stageWrapRef.value) _stageFitObserver.observe(stageWrapRef.value)
  }
  // v0.7.132 — 监听 devicePixelRatio 变化（拖窗口到不同 DPR 屏时触发）
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    const armDprListener = () => {
      const mq = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`)
      const onChange = () => {
        applyKonvaHiDPI(stageRef.value?.getStage?.() || (window as any).__wkStage)
        fitStage()
      }
      mq.addEventListener('change', onChange)
    }
    armDprListener()
  }
  // v0.7.136 — 字体加载完成时重绘画布（参考 genoffice SlideCanvas.tsx:555-565）
  // PPTX 中可能包含自定义字体（如 Oranienbaum、Liter、MiSans），首次渲染时
  // canvas text 会用 fallback 字体绘制；等真字体到位后必须重绘一次。
  if (typeof document !== 'undefined' && (document as any).fonts) {
    const fontsApi = (document as any).fonts
    const redrawAfterFontLoad = () => {
      // v0.7.151 — 字体加载完成后只 batchDraw 不够，必须重新应用 DPR（pixelRatio）
      // 并强制重画每个 layer。Canvas text 在字体到位前会画 fallback，画完之后必须
      // 重新 rasterize 才不会显示成"空白"。
      kickRedraw()
    }
    fontsApi.ready?.then?.(redrawAfterFontLoad)?.catch?.(() => {})
    fontsApi.addEventListener?.('loadingdone', redrawAfterFontLoad)
  }
})
onBeforeUnmount(() => { if (_stageFitObserver) _stageFitObserver.disconnect() })
watch([stageWidthPx, stageHeightPx, activeSlide], () => nextTick(() => { fitStage(); kickRedraw() }))
// v0.7.151 — stageRef 变化后 fitStage + kickRedraw，确保 v-layer 第一次插入时
// layer canvas 也拿到正确的 pixelRatio 并重画一次。
watch(stageRef, () => nextTick(() => { fitStage(); kickRedraw() }), { flush: 'post' })
// v0.7.136 — Vue-Konva 在 PPTX 异步加载完后，v-for 添加的 shapes 经常不会触发
// Konva 的自动重绘，导致 canvas 看起来是空白背景色。手动监听 activeShapes 长度
// 变化，强制 layer.batchDraw()。参考 genoffice SlideCanvas.tsx:555-565 的字体监听模式。
watch(activeShapes, (next, prev) => {
  if (next === prev || !Array.isArray(next)) return
  if (next.length === (prev?.length ?? 0)) {
    // v0.7.151 — shapes 数量没变但内容变了（Yjs 远端 patch / 编辑器内部修改），同样
    // 需要触发 layer 重画，否则字体会停在旧版本上、Canvas 内容看起来没动。
    nextTick(() => kickRedraw())
    return
  }
  nextTick(() => kickRedraw())
}, { flush: 'post', deep: true })

// v0.7.137 — font fallback chain for text shapes. The PPTX may declare
// Oranienbaum / Liter / MiSans etc, none of which exist on macOS. We append
// system CJK-capable fonts (PingFang SC, Microsoft YaHei, Hiragino Sans GB,
// MiSans, WenQuanYi Micro Hei) so that Chinese text actually renders instead
// of disappearing as blank rectangles.
const textFontFamily = (shapeFont: string | undefined) => {
  // v0.7.148 — 把 system-ui / -apple-system 放到链首，省掉浏览器先尝试不存在
  // 的 MiSans 然后才回退到系统字体这段时间里的"白方块/灰条"渲染。
  const chain = 'system-ui, -apple-system, BlinkMacSystemFont, \'Helvetica Neue\', Helvetica, \'PingFang SC\', \'Hiragino Sans GB\', \'Microsoft YaHei\', \'WenQuanYi Micro Hei\', \'MiSans\', sans-serif'
  return shapeFont ? shapeFont + ', ' + chain : chain
}

// v0.7.152 — contrast-aware text fill. PPTX 解析出的 shape.fontColor 经常
// 是 undefined（PPTX 默认色被省略），旧实现 fallback 到 #f8fafc 在 #FFFFFF
// 背景上几乎不可见，导致「PPT 渲染空白」。现在根据 slide 背景亮度自动选
// 字色：浅底（luminance >= 0.4）→ 深字 #0f172a；深底 → 浅字 #f8fafc。
const defaultTextFill = computed(() =>
  luminance(activeSlide.value?.background) < 0.4 ? '#f8fafc' : '#0f172a',
)

// v0.7.199 — vue-konva does not propagate reactive `defaultTextFill` updates
// to Konva.Text.fill() because the inline :config object literal keeps the
// same string each render. Without this watcher, a dark slide (luminance < 0.4)
// renders text in the default Konva "black", invisible against the dark
// backdrop. Force-set fill on every Konva Text shape whenever the active
// slide changes (covers initial load + slide switching + bg changes).
const syncTextFillsToKonva = () => {
  const stage = stageRef.value?.getStage?.() || stageRef.value
  if (!stage || typeof stage.getLayers !== 'function') return
  const want = defaultTextFill.value
  for (const layer of stage.getLayers()) {
    for (const node of layer.children || []) {
      if (node && node.className === 'Text' && node.fill() !== want) {
        node.fill(want)
      }
    }
    layer.batchDraw()
  }
}
watch([defaultTextFill, activeShapes], () => {
  nextTick(() => syncTextFillsToKonva())
}, { flush: 'post' })


// --- Thumbnail SVG rendering (real shapes, scaled to a 132-wide stage) ---
const thumbViewport = { w: SLIDE_W_INCH * 96, h: SLIDE_H_INCH * 96 }
const thumbViewBox = `0 0 ${thumbViewport.w} ${thumbViewport.h}`
const thumbScale = computed(() => 132 / thumbViewport.w)
const thumbTriangle = (w: number, h: number) => `0,${h} ${w/2},0 ${w},${h}`
const thumbStar = (w: number, h: number) => {
  const cx = w / 2, cy = h / 2
  const r = Math.min(w, h) / 2
  const inner = r * 0.42
  const pts: string[] = []
  for (let i = 0; i < 10; i++) {
    const a = -Math.PI / 2 + (i * Math.PI) / 5
    const rad = i % 2 === 0 ? r : inner
    pts.push(`${cx + Math.cos(a) * rad},${cy + Math.sin(a) * rad}`)
  }
  return pts.join(' ')
}
const thumbHexagon = (w: number, h: number) => {
  const off = w * 0.25
  return [
    `${off},0`,
    `${w - off},0`,
    `${w},${h/2}`,
    `${w - off},${h}`,
    `${off},${h}`,
    `0,${h/2}`,
  ].join(' ')
}

// v0.7.200 — 缩略图 SVG 文字颜色随 slide 背景亮度自动反转 (深底浅字 / 浅底深字)。
// 之前硬编码 #0f172a, 在 #1E1E1E / #043F5A 等深色 slide 缩略图上完全不可见,
// 复刻了 v0.7.199 修过的 Konva canvas 文字不可见 bug。
const thumbTextFill = (bg: string | undefined): string => {
  if (!bg) return '#0f172a'
  return luminance(bg) < 0.4 ? '#f8fafc' : '#0f172a'
}

// --- Konva stage / transformer wiring ---
const publishCursor = (px: number, py: number) => {
  if (!handle) return
  // px / py are CSS pixels; convert back to EMU
  const xEmu = Math.round((px / PX_PER_INCH) * 914400)
  const yEmu = Math.round((py / PX_PER_INCH) * 914400)
  handle.provider.awareness.setLocalStateField('cursor', { slide: activeIndex.value, x: xEmu, y: yEmu })
}
/** Publish selection: which shapes (if any) the user has selected on the
 *  current slide. Other collaborators see colored dashed outlines around
 *  the same shapes (rendered via `remoteSelections`). */
const publishSelection = (shapeIds: string[]) => {
  if (!handle) return
  handle.provider.awareness.setLocalStateField('selection', {
    slide: activeIndex.value,
    shapeId: shapeIds[shapeIds.length - 1] ?? '',
    shapeIds,
  })
}
const transformerRef = ref<any>(null)

// v0.7.107 — always expose Konva stage / transformer / selection setter once the
// Vue-Konva components mount. Previously __wkStage was only set inside
// updateTransformer(), which fires only after a Konva event — leaving E2E
// scripts (and any external automation) unable to drive the editor until a
// user click happens. Watching the refs with `immediate: ===` covers both
// orderings: ref-undefined-then-set and ref-already-set-on-mount.
watch([stageRef, transformerRef], ([s, t]) => {
  if (typeof window === 'undefined') return
  const stage = s?.getStage?.() ?? null
  const tr = t?.getNode?.() ?? null
  if (stage) (window as any).__wkStage = stage
  if (tr) (window as any).__wkTransformer = tr
  ;(window as any).__wkSlideSelection = (ids: string[]) => {
    selectedIds.value = ids.slice()
    selectedId.value = ids.length ? ids[ids.length - 1] : null
  }
}, { immediate: true })

// --- v0.7.29 — comments anchor (current slide + selected shape if any) ---
const commentAnchor = ref<{ type: 'doc' | 'slide' | 'sheet'; ref: string } | null>(null)
watch([activeIndex, selectedId, selectedIds], ([s, id, ids]) => {
  commentAnchor.value = {
    type: 'slide',
    ref: JSON.stringify({ slide: s, shapeId: id ?? '' }),
  }
  publishSelection(ids.length ? ids : (id ? [id] : []))
}, { immediate: true })

/** Set the selection to exactly one shape (or clear it). */
const selectOnly = (id: string | null) => {
  selectedId.value = id
  selectedIds.value = id ? [id] : []
}

const onStageClick = (e: any) => {
  const stage = stageRef.value?.getStage?.()
  // Click on empty stage clears selection.
  const target = e?.target
  if (!target || target === stage) {
    selectOnly(null)
    updateTransformer()
  }
  if (stage) {
    const pos = stage.getPointerPosition?.()
    if (pos) publishCursor(pos.x, pos.y)
  }
}

const onShapeClick = (id: string, e: any) => {
  const multi = e?.evt?.shiftKey || e?.evt?.ctrlKey || e?.evt?.metaKey
  if (multi) {
    const set = new Set(selectedIds.value)
    if (set.has(id)) set.delete(id)
    else set.add(id)
    selectedIds.value = [...set]
    selectedId.value = id
  } else {
    selectOnly(id)
    // v0.7.104 — coalesce all shapes that share the clicked shape's groupId
    // so users can move / resize a group by clicking any member.
    const target = activeShapes.value.find((s) => s.id === id)
    const gid = target?.groupId
    if (gid) {
      const mates = activeShapes.value.filter((s) => s.groupId === gid).map((s) => s.id)
      selectedIds.value = mates
      selectedId.value = id
    }
  }
  updateTransformer()
}

const updateTransformer = async () => {
  await nextTick()
  const stage = stageRef.value?.getStage?.()
  const tr = transformerRef.value?.getNode?.()
  // v0.7.104 — E2E hook: expose stage + transformer so Playwright can read
  // anchor screen coords directly (avoids guessing from EMU math).
  // (The actual exposure now lives in the top-level watch on stageRef /
  // transformerRef so the hooks are available even before any user click —
  // see v0.7.107. The assignments below keep the late-binding case in sync.)
  if (typeof window !== 'undefined') {
    if (stage) (window as any).__wkStage = stage
    if (tr) (window as any).__wkTransformer = tr
  }
  if (!stage || !tr) return
  if (!selectedId.value && selectedIds.value.length === 0) {
    tr.nodes([])
    tr.getLayer()?.batchDraw?.()
    return
  }
  // v0.7.101/104 — bind the transformer to every selected shape so users
  // can resize a multi-selection (or an entire group) with a single handle.
  const ids = selectedIds.value.length ? selectedIds.value : (selectedId.value ? [selectedId.value] : [])
  const nodes = ids.map((id) => stage.findOne(`#${id}`)).filter(Boolean) as any[]
  if (nodes.length) {
    tr.nodes(nodes)
    tr.getLayer()?.batchDraw?.()
  } else {
    tr.nodes([])
    tr.getLayer()?.batchDraw?.()
  }
}

const onShapeDragEnd = (id: string, e: any) => {
  const node = e?.target
  if (!node) return
  const newX = Math.round((node.x() / PX_PER_INCH) * 914400)
  const newY = Math.round((node.y() / PX_PER_INCH) * 914400)
  // v0.7.104 — Konva moves all multi-selected nodes together; mirror the
  // delta onto each peer so the saved model stays in sync.
  const ids = selectedIds.value.length ? selectedIds.value : [id]
  const source = activeShapes.value.find((s) => s.id === id)
  if (source && ids.length > 1) {
    const dx = newX - source.x
    const dy = newY - source.y
    const patches: Array<{ id: string; patch: Partial<PptxShape> }> = []
    for (const sid of ids) {
      const s = activeShapes.value.find((sh) => sh.id === sid)
      if (!s) continue
      patches.push({ id: sid, patch: { x: s.x + dx, y: s.y + dy } })
    }
    updateShapes(patches)
    return
  }
  updateShape(id, { x: newX, y: newY })
}

const onShapeTransformEnd = (id: string, e: any) => {
  const node = e?.target
  if (!node) return
  const scaleX = node.scaleX()
  const scaleY = node.scaleY()
  const ids = selectedIds.value.length ? selectedIds.value : [id]
  const isMulti = ids.length > 1

  // Reset scales for every dragged node before baking them into width/height.
  // Each node carries its own scale from Konva (they all move together).
  const stage = stageRef.value?.getStage?.()
  if (isMulti && stage) {
    const peerNodes = ids.map((sid) => stage.findOne(`#${sid}`)).filter(Boolean) as any[]
    for (const pn of peerNodes) {
      // Bake scale into the model before resetting Konva's scale.
      const sx = pn.scaleX()
      const sy = pn.scaleY()
      pn.scaleX(1)
      pn.scaleY(1)
      const newW = Math.round((pn.width() * sx / PX_PER_INCH) * 914400)
      const newH = Math.round((pn.height() * sy / PX_PER_INCH) * 914400)
      const newX = Math.round((pn.x() / PX_PER_INCH) * 914400)
      const newY = Math.round((pn.y() / PX_PER_INCH) * 914400)
      const sh = activeShapes.value.find((s) => s.id === pn.id())
      if (!sh) continue
      updateShape(pn.id(), { x: newX, y: newY, w: newW, h: newH })
    }
    updateTransformer()
    return
  }

  // Single-node path (legacy behaviour preserved).
  node.scaleX(1)
  node.scaleY(1)
  const newW = Math.round((node.width() * scaleX / PX_PER_INCH) * 914400)
  const newH = Math.round((node.height() * scaleY / PX_PER_INCH) * 914400)
  const newX = Math.round((node.x() / PX_PER_INCH) * 914400)
  const newY = Math.round((node.y() / PX_PER_INCH) * 914400)
  updateShape(id, { x: newX, y: newY, w: newW, h: newH })
  updateTransformer()
}

const onTextEdit = (id: string, _e: any) => {
  // Promote to inspector edit mode — minimal but works.
  selectOnly(id)
}

// --- Inspector inputs ---
const inspectorText = ref('')
const inspectorFill = ref('')
const inspectorStroke = ref('')
const inspectorFontSize = ref(18)
// v0.7.38 — extended format panel
const inspectorStrokeWidth = ref(1)
const inspectorBold = ref(false)
const inspectorItalic = ref(false)

// Helpers for color picker <-> hex round-trip (picker needs 6-char #rrggbb).
const inspectorFillColor = computed(() => (inspectorFill.value || '000000').padStart(6, '0').slice(-6).padStart(6, '0').replace(/^(.{6})$/, '#$1'))
const inspectorStrokeColor = computed(() => (inspectorStroke.value || '000000').padStart(6, '0').slice(-6).padStart(6, '0').replace(/^(.{6})$/, '#$1'))

const onInspectorFillPicker = (v: string) => {
    inspectorFill.value = v.replace(/^#/, '')
    onInspectorFillChange()
}
const onInspectorStrokePicker = (v: string) => {
    inspectorStroke.value = v.replace(/^#/, '')
    onInspectorStrokeChange()
}
const onInspectorStrokeWidthChange = () => updateShape(selectedId.value!, { strokeWidth: inspectorStrokeWidth.value })
// v0.7.79 — inspector rotation input
const inspectorRotation = ref(0)
watch(selectedShape, (s) => {
  inspectorRotation.value = Math.round(s?.rotation ?? 0)
})
const onInspectorRotationChange = () => {
  if (!selectedId.value) return
  const next = normalizeRotation(Number(inspectorRotation.value) || 0)
  updateShape(selectedId.value, { rotation: next } as any)
}
const toggleBold = () => { inspectorBold.value = !inspectorBold.value; updateShape(selectedId.value!, { bold: inspectorBold.value } as any) }
const toggleItalic = () => { inspectorItalic.value = !inspectorItalic.value; updateShape(selectedId.value!, { italic: inspectorItalic.value } as any) }

// v0.7.38 Build #46.x — slide animations (entrance / emphasis / exit).
const animations = ref<SlideAnimationRecord[]>([])
const newEffect = ref<AnimEffectKind>('fade')
const newTrigger = ref<AnimTrigger>('onClick')

const effectLabel = (e: AnimEffectKind): string => {
  const map: Record<AnimEffectKind, string> = {
    fade: '淡入', flyIn: '飞入', zoom: '缩放', spin: '旋转', bounce: '弹跳',
    appear: '出现', disappear: '消失', pulse: '脉冲', colorPulse: '变色脉冲',
    teeter: '摇摆', growShrink: '缩放',
  }
  return map[e] || e
}

const triggerLabel = (t: AnimTrigger): string => {
  const map: Record<AnimTrigger, string> = {
    onClick: '点击时', withPrevious: '同时', afterPrevious: '之后',
  }
  return map[t] || t
}

// v0.7.112 — per-animation edit dropdowns share these option lists.
const animEffectOptions: Array<{ value: AnimEffectKind; label: string }> = [
  { value: 'fade', label: '淡入' },
  { value: 'flyIn', label: '飞入' },
  { value: 'zoom', label: '缩放' },
  { value: 'spin', label: '旋转' },
  { value: 'bounce', label: '弹跳' },
  { value: 'appear', label: '出现' },
  { value: 'disappear', label: '消失' },
  { value: 'pulse', label: '脉冲' },
  { value: 'colorPulse', label: '变色脉冲' },
  { value: 'teeter', label: '摇摆' },
  { value: 'growShrink', label: '缩放' },
]
const animTriggerOptions: Array<{ value: AnimTrigger; label: string }> = [
  { value: 'onClick', label: '点击时' },
  { value: 'withPrevious', label: '与上一动画同时' },
  { value: 'afterPrevious', label: '上一动画之后' },
]

const refreshAnimations = () => {
  if (!deck.value || !deck.value.opened) {
    animations.value = []
    return
  }
  animations.value = getSlideAnimationsOnDeck(deck.value, activeIndex.value)
}

watch(activeIndex, () => {
  refreshAnimations()
  loadTransitionForActive()
  // v0.7.101 — clear selection when switching slides (ids are slide-local).
  selectOnly(null)
})
watch(() => deck.value?.opened, () => {
  refreshAnimations()
  loadTransitionForActive()
}, { immediate: false })

const addAnimation = () => {
  if (!deck.value || selectedId.value == null) return
  const spId = getShapeSpIdOnDeck(deck.value, activeIndex.value, selectedId.value)
  if (spId == null) {
    MessagePlugin.warning('无法为所选形状添加动画:缺少 spId')
    return
  }
  const next: SlideAnimationRecord[] = [
    ...animations.value,
    { spId, effect: newEffect.value, trigger: newTrigger.value, durationMs: 1000, delayMs: 0 },
  ]
  if (setSlideAnimationsOnDeck(deck.value, activeIndex.value, next)) {
    animations.value = next
    scheduleSave()
  }
}

const removeAnimation = (idx: number) => {
  if (!deck.value) return
  const next = animations.value.filter((_, i) => i !== idx)
  if (setSlideAnimationsOnDeck(deck.value, activeIndex.value, next)) {
    animations.value = next
    scheduleSave()
  }
}

const clearAnimations = () => {
  if (!deck.value) return
  if (setSlideAnimationsOnDeck(deck.value, activeIndex.value, [])) {
    animations.value = []
    scheduleSave()
  }
}

// v0.7.112 — per-animation edit handlers: patch one field, then refresh.
const onAnimationPatch = (
  idx: number,
  key: 'effect' | 'trigger' | 'durationMs' | 'delayMs',
  value: string | number,
) => {
  if (!deck.value) return
  const patch: Record<string, string | number> = {}
  patch[key] = value
  if (patchSlideAnimationOnDeck(deck.value, activeIndex.value, idx, patch)) {
    refreshAnimations()
    scheduleSave()
  }
}

// v0.7.112 — move an animation up (-1) / down (+1) within the slide list.
const moveAnimation = (idx: number, dir: -1 | 1) => {
  if (!deck.value) return
  if (reorderSlideAnimationOnDeck(deck.value, activeIndex.value, idx, dir)) {
    refreshAnimations()
    scheduleSave()
  }
}

// v0.7.113 — PPT 母版视图 (genoffice vendor): list master + layouts, preview XML,
// rename via <p:cSld name>. Read-only edit for now; richer Konva editing is
// tracked under v0.7.113.x.
const masterModalOpen = ref(false)
const masterParts = ref<EngineMasterPartInfo[]>([])
const selectedMasterIdx = ref(-1)
const masterNameDraft = ref('')
const masterPreviewXml = ref('')
const masterFeedback = ref<string | null>(null)
const masterElementSummary = ref('')

const selectedMaster = computed<EngineMasterPartInfo | null>(() => {
  return selectedMasterIdx.value >= 0 && selectedMasterIdx.value < masterParts.value.length
    ? masterParts.value[selectedMasterIdx.value] ?? null
    : null
})
const masterNameDirty = computed(() => {
  const cur = selectedMaster.value?.name ?? ''
  return (masterNameDraft.value ?? '').trim() !== cur.trim()
})

const summarizeElementKinds = (slide: Slide | null): string => {
  if (!slide) return ''
  const kinds: Record<string, number> = {}
  for (const el of slide.elements ?? []) {
    const k = (el as { type?: string }).type ?? 'unknown'
    kinds[k] = (kinds[k] ?? 0) + 1
  }
  const total = slide.elements?.length ?? 0
  return total + ' (' + Object.entries(kinds).map(([k, v]) => k + ':' + v).join(', ') + ')'
}

const openMasterModal = () => {
  if (!deck.value) return
  masterParts.value = listMasterPartsOnDeck(deck.value)
  selectedMasterIdx.value = masterParts.value.length > 0 ? 0 : -1
  masterFeedback.value = null
  refreshSelectedMaster()
  masterModalOpen.value = true
}

const closeMasterModal = () => {
  masterModalOpen.value = false
  selectedMasterIdx.value = -1
  masterParts.value = []
  masterPreviewXml.value = ''
  masterElementSummary.value = ''
  masterNameDraft.value = ''
  masterFeedback.value = null
}

const refreshSelectedMaster = () => {
  const p = selectedMaster.value
  if (!p || !deck.value) {
    masterPreviewXml.value = ''
    masterElementSummary.value = ''
    return
  }
  const xml = readMasterPartXmlOnDeck(deck.value, p.partPath)
  masterPreviewXml.value = xml ? (xml.length > 1200 ? xml.slice(0, 1200) + '\n…' : xml) : ''
  const slide = parseMasterToSlideOnDeck(deck.value, p.partPath)
  masterElementSummary.value = summarizeElementKinds(slide)
  masterNameDraft.value = p.name ?? ''
}

const selectMasterPart = (idx: number) => {
  selectedMasterIdx.value = idx
  masterFeedback.value = null
  refreshSelectedMaster()
}

const applyMasterRename = () => {
  if (!deck.value || !selectedMaster.value) return
  const target = selectedMaster.value
  const newName = masterNameDraft.value.trim()
  const ok = renameMasterOnDeck(deck.value, target.partPath, newName)
  if (!ok) {
    masterFeedback.value = '重命名失败 (XML 未变化)'
    return
  }
  // Refresh master list (engine will see new cSld name next call)
  masterParts.value = listMasterPartsOnDeck(deck.value)
  selectedMasterIdx.value = masterParts.value.findIndex((p) => p.partPath === target.partPath)
  refreshSelectedMaster()
  scheduleSave()
  MessagePlugin.success('母版名称已暂存（保存 .pptx 时落盘）')
}

watch(selectedShape, (s) => {
  if (!s) return
  inspectorText.value = s.text ?? ''
  inspectorFill.value = s.fill ?? ''
  inspectorStroke.value = s.stroke ?? ''
  inspectorFontSize.value = s.fontSize ?? 18
  inspectorStrokeWidth.value = (s as any).strokeWidth ?? 1
  inspectorBold.value = Boolean((s as any).bold)
  inspectorItalic.value = Boolean((s as any).italic)
})
const onInspectorTextChange = () => updateShape(selectedId.value!, { text: inspectorText.value })
const onInspectorFillChange = () => updateShape(selectedId.value!, { fill: inspectorFill.value.replace(/^#/, '') })
const onInspectorStrokeChange = () => updateShape(selectedId.value!, { stroke: inspectorStroke.value.replace(/^#/, '') })
const onInspectorFontSizeChange = () => updateShape(selectedId.value!, { fontSize: inspectorFontSize.value })

// --- Yjs shape sync ---
const shapeToObj = (s: PptxShape): Record<string, unknown> => ({
  id: s.id,
  type: s.type,
  x: s.x, y: s.y, w: s.w, h: s.h,
  text: s.text ?? '',
  fill: s.fill ?? '',
  stroke: s.stroke ?? '',
  strokeWidth: s.strokeWidth ?? 0,
  fontSize: s.fontSize ?? 18,
  mediaRef: s.mediaRef ?? '',
  mediaData: s.mediaData ?? '',
  spIndex: s.spIndex,
  sourceType: s.sourceType ?? '',
  preset: s.preset ?? '',
  rows: s.rows ?? 0,
  cols: s.cols ?? 0,
  cellTexts: JSON.stringify(s.cellTexts ?? []),
})

const objToShape = (o: Record<string, unknown>): PptxShape => {
  const color = (value: unknown): string | undefined => {
    if (!value) return undefined
    return String(value).replace(/^#/, '') || undefined
  }
  const finiteNumber = (value: unknown, fallback: number) => {
    const number = Number(value)
    return Number.isFinite(number) ? number : fallback
  }
  let cellTexts: string[][] | undefined
  if (o.cellTexts) {
    if (typeof o.cellTexts === 'string') {
      try { cellTexts = JSON.parse(o.cellTexts) } catch { cellTexts = undefined }
    } else if (Array.isArray(o.cellTexts)) {
      cellTexts = (o.cellTexts as unknown[][]).map((r) => Array.isArray(r) ? (r as unknown[]).map(String) : [String(r)])
    }
  }
  return {
    id: String(o.id ?? ''),
    type: (o.type ?? 'text') as PptxShape['type'],
    x: finiteNumber(o.x, 0),
    y: finiteNumber(o.y, 0),
    w: Math.max(1, finiteNumber(o.w, 914400)),
    h: Math.max(1, finiteNumber(o.h, 457200)),
    text: o.text ? String(o.text) : undefined,
    fill: color(o.fill),
    stroke: color(o.stroke),
    strokeWidth: finiteNumber(o.strokeWidth, 0) || undefined,
    fontSize: Math.max(1, finiteNumber(o.fontSize, 18)),
    mediaRef: o.mediaRef ? String(o.mediaRef) : undefined,
    mediaData: o.mediaData ? String(o.mediaData) : undefined,
    spIndex: Number(o.spIndex ?? -1),
    sourceType: o.sourceType ? String(o.sourceType) : undefined,
    preset: o.preset ? String(o.preset) : undefined,
    rows: o.rows ? finiteNumber(o.rows, 0) : undefined,
    cols: o.cols ? finiteNumber(o.cols, 0) : undefined,
    cellTexts,
    // v0.7.104 — group id mirror
    groupId: o.groupId ? String(o.groupId) : undefined,
  }
}

// --- v0.7.98 — shape alignment to slide bounds ---
// v0.7.101 — multi-select: align to bounding box, distribute, match size.
type AlignDirection = 'left' | 'centerH' | 'right' | 'top' | 'centerV' | 'bottom'
const selectedShapes = computed(() => {
  const slide = activeSlide.value
  if (!slide) return []
  const ids = new Set(selectedIds.value.length ? selectedIds.value : (selectedId.value ? [selectedId.value] : []))
  return slide.shapes.filter((s) => ids.has(s.id))
})

const alignSelected = (direction: AlignDirection) => {
  const shapes = selectedShapes.value
  if (!shapes.length) return
  const slide = activeSlide.value!
  const sw = slide.width ?? SLIDE_W_INCH * 914400
  const sh = slide.height ?? SLIDE_H_INCH * 914400
  // Single selection aligns to slide bounds; multi aligns to bounding box.
  let container: { x: number; y: number; w: number; h: number }
  if (shapes.length === 1) {
    container = { x: 0, y: 0, w: sw, h: sh }
  } else {
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    for (const s of shapes) {
      minX = Math.min(minX, s.x); minY = Math.min(minY, s.y)
      maxX = Math.max(maxX, s.x + s.w); maxY = Math.max(maxY, s.y + s.h)
    }
    container = { x: minX, y: minY, w: maxX - minX, h: maxY - minY }
  }
  const patches: Array<{ id: string; patch: Partial<PptxShape> }> = []
  for (const s of shapes) {
    let nx = s.x
    let ny = s.y
    switch (direction) {
      case 'left': nx = container.x; break
      case 'centerH': nx = Math.round(container.x + (container.w - s.w) / 2); break
      case 'right': nx = container.x + container.w - s.w; break
      case 'top': ny = container.y; break
      case 'centerV': ny = Math.round(container.y + (container.h - s.h) / 2); break
      case 'bottom': ny = container.y + container.h - s.h; break
    }
    if (nx !== s.x || ny !== s.y) patches.push({ id: s.id, patch: { x: nx, y: ny } })
  }
  updateShapes(patches)
}

// v0.7.101 — horizontal/vertical equal spacing (≥3 shapes).
const distributeSelected = (axis: 'h' | 'v') => {
  const shapes = selectedShapes.value
  if (shapes.length < 3) return
  const indexed = shapes
    .map((s, i) => ({ s, i }))
    .sort((a, b) => (axis === 'h' ? a.s.x - b.s.x : a.s.y - b.s.y))
  const first = indexed[0]!
  const last = indexed[indexed.length - 1]!
  const totalSpan = axis === 'h' ? last.s.x + last.s.w - first.s.x : last.s.y + last.s.h - first.s.y
  const totalSize = indexed.reduce((sum, { s }) => sum + (axis === 'h' ? s.w : s.h), 0)
  const gap = (totalSpan - totalSize) / (indexed.length - 1)
  const patches: Array<{ id: string; patch: Partial<PptxShape> }> = []
  let cursor = axis === 'h' ? first.s.x : first.s.y
  for (const { s, i } of indexed) {
    patches.push({ id: s.id, patch: (axis === 'h' ? { x: Math.round(cursor) } : { y: Math.round(cursor) }) as Partial<PptxShape> })
    cursor += (axis === 'h' ? s.w : s.h) + gap
  }
  updateShapes(patches)
}

// v0.7.101 — match width/height to the primary (last-clicked) selected shape.
// v0.7.119 — flip horizontal / vertical (toggle shape.transform.flipH/flipV).
const flipSelected = (axis: 'h' | 'v') => {
  const shapes = selectedShapes.value
  if (!shapes.length || !ydeck) return
  ydeck.doc?.transact(() => {
    const yslide = ydeck!.get(activeIndex.value) as Y.Map<unknown> | undefined
    if (!yslide) return
    const yshapes = yslide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    if (!yshapes) return
    const arr = yshapes.toArray()
    const slide = deck.value?.opened?.deck.slides[activeIndex.value]
    const patches: Array<{ id: string; patch: Partial<PptxShape> }> = []
    arr.forEach((m, i) => {
      const id = m.get('id') as string
      if (!shapes.find((s) => s.id === id)) return
      const el: any = slide?.elements.find((e: any) => e.id === id)
      if (!el) return
      el.transform = el.transform || { offset: { x: 0, y: 0, cx: 0, cy: 0 }, rot: 0, flipH: false, flipV: false }
      if (axis === 'h') el.transform.flipH = !el.transform.flipH
      else el.transform.flipV = !el.transform.flipV
      slide.structureDirty = true
      patches.push({ id, patch: {} as Partial<PptxShape> })
    })
    if (slide) flushStructure(slide)
    if (patches.length) updateShapes(patches)
    scheduleSave()
  })
}

// v0.7.119 — group bbox: convenience for present mode entrypoint.
/**
 * v0.7.188 — 「切换」/「动画」tab 点开后：展开底部 .collab-slide-konva__animations
 * 面板并滚动到它可见。GenOffice 风格是动画面板常驻右侧；这里走"折中"
 * 路线——保留现有底部面板，只在用户主动想用时弹出来。
 */
const onOpenAnimationsPanel = () => {
  panelsCollapsed.animations = false
  nextTick(() => {
    const el = document.querySelector('.collab-slide-konva__animations')
    el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  })
}

const onEnterPresentFromStart = () => {
  if (!slides.value.length || loading.value) return
  presentIndex.value = 0
  presentMode.value = true
}

const matchSize = (axis: 'w' | 'h') => {
  const shapes = selectedShapes.value
  if (shapes.length < 2) return
  const ref = shapes.find((s) => s.id === selectedId.value) ?? shapes[0]
  const patches: Array<{ id: string; patch: Partial<PptxShape> }> = []
  for (const s of shapes) {
    if (s.id === ref.id) continue
    if (axis === 'w' && s.w !== ref.w) patches.push({ id: s.id, patch: { w: ref.w } })
    if (axis === 'h' && s.h !== ref.h) patches.push({ id: s.id, patch: { h: ref.h } })
  }
  updateShapes(patches)
}

// v0.7.104 — group / ungroup
// Returns the common groupId for the current selection if all selected shapes
// share a non-empty groupId; otherwise returns null.
const selectedShapeGroupId = computed<string | null>(() => {
  const shapes = selectedShapes.value
  if (!shapes.length) return null
  const ids = shapes.map((s) => s.groupId || '').filter((g) => !!g)
  if (ids.length !== shapes.length) return null
  const first = ids[0]
  return ids.every((g) => g === first) ? first : null
})

// Group button is enabled when 2+ shapes are selected AND none of them is already in a group.
const canGroupSelected = computed(() => {
  const shapes = selectedShapes.value
  if (shapes.length < 2) return false
  return shapes.every((s) => !s.groupId)
})

// Ungroup button is enabled when all selected shapes share a single groupId.
const canUngroupSelected = computed(() => !!selectedShapeGroupId.value)

const makeGroupId = () => `g_${Math.random().toString(36).slice(2, 10)}_${Date.now().toString(36)}`

// Group every selected shape under a new shared groupId. Same Yjs transact
// path as updateShapes so CRDT peers converge in one transaction.
// v0.7.107 — also call engine's groupElements() so the engine's slide.elements
// gets a real <p:grpSp> replacing the N children. The save path (engineSavePptx)
// then emits the grpSp into ppt/slides/slideN.xml without further wiring.
const groupSelected = () => {
  const shapes = selectedShapes.value
  if (shapes.length < 2) return
  if (shapes.some((s) => !!s.groupId)) return
  const gid = makeGroupId()
  const patches: Array<{ id: string; patch: Partial<PptxShape> }> = shapes.map((s) => ({ id: s.id, patch: { groupId: gid } }))
  updateShapes(patches)
  // Mutate the engine's slide model so savePptx re-emits a real <p:grpSp>.
  // We collect sourceIds from the Yjs-side shape ids; engineGroupElements
  // resolves them against slide.elements via matchesElementRef (which handles
  // both Yjs-side and engine-side ids).
  if (deck.value?.opened) {
    const opened = deck.value.opened
    const slideIdx = activeIndex.value
    const sourceIds = shapes.map((s) => s.id)
    try {
      const result = groupElements(opened, slideIdx, sourceIds)
      if (result) engineGroupIdByYjsGroupId[gid] = result.groupId
    } catch (e) {
      // eslint-disable-next-line no-console
      console.warn('[CollabSlideKonvaEditor] engine groupElements failed', e)
    }
    // v0.7.108 — record this gid as already projected so the very next
    // syncFromY (fired by the local Yjs observe) does not double-trigger
    // groupElements for the same set of sourceIds.
    markLocalGrouped(lastSyncedYjsGroupIdsBySlideIdx, slideIdx, gid)
  }
}

// Ungroup: clear groupId on every selected shape that currently belongs to the
// selection's shared groupId. Shapes from a different group are left alone
// (the only way they're in selectedShapes is if onShapeClick coalesced a group
// into the selection, which it does — so this matches user expectation).
// v0.7.107 — also call engine's ungroupElement() so the engine's grpSp is
// replaced by its lifted children in slide.elements, and savePptx emits
// independent <p:sp> elements again.
const ungroupSelected = () => {
  const shapes = selectedShapes.value
  if (!shapes.length) return
  const gid = selectedShapeGroupId.value
  if (!gid) return
  const patches: Array<{ id: string; patch: Partial<PptxShape> }> = shapes
    .filter((s) => s.groupId === gid)
    .map((s) => ({ id: s.id, patch: { groupId: '' } }))
  if (!patches.length) return
  updateShapes(patches)
  if (deck.value?.opened) {
    const engineGid = engineGroupIdByYjsGroupId[gid]
    const slideIdx = activeIndex.value
    if (engineGid) {
      try {
        ungroupElement(deck.value.opened, slideIdx, engineGid)
      } catch (e) {
        // eslint-disable-next-line no-console
        console.warn('[CollabSlideKonvaEditor] engine ungroupElement failed', e)
      }
      delete engineGroupIdByYjsGroupId[gid]
    }
    // v0.7.108 — drop the gid from the projection set so the local
    // syncFromY observes no diff and does not call ungroupElement again.
    markLocalUngrouped(lastSyncedYjsGroupIdsBySlideIdx, slideIdx)
  }
}

/** Batch-update several shapes in a single Yjs transaction. */
const updateShapes = (patches: Array<{ id: string; patch: Partial<PptxShape> }>) => {
  if (!patches.length || !ydeck || !activeSlide.value) return
  const slideIdx = activeIndex.value
  ydeck.doc?.transact(() => {
    const yslide = ydeck!.get(slideIdx) as Y.Map<unknown> | undefined
    if (!yslide) return
    const yshapes = yslide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    if (!yshapes) return
    const arr = yshapes.toArray()
    for (const { id, patch } of patches) {
      const i = arr.findIndex((m) => m.get('id') === id)
      if (i < 0) continue
      const m = arr[i]
      for (const [k, v] of Object.entries(patch)) {
        if (v !== undefined) m.set(k, v as any)
      }
      markDirty(id, patch)
    }
    scheduleSave()
  })
}

const updateShape = (id: string, patch: Partial<PptxShape>) => {
  if (!ydeck || !activeSlide.value) return
  const slideIdx = activeIndex.value
  ydeck.doc?.transact(() => {
    const yslide = ydeck!.get(slideIdx) as Y.Map<unknown> | undefined
    if (!yslide) return
    const yshapes = yslide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    if (!yshapes) return
    const arr = yshapes.toArray()
    const i = arr.findIndex((m) => m.get('id') === id)
    if (i < 0) return
    const m = arr[i]
    for (const [k, v] of Object.entries(patch)) {
      if (v !== undefined) m.set(k, v as any)
    }
    markDirty(id, patch)
    scheduleSave()
  })
}

const markDirty = (id: string, patch: Partial<PptxShape>) => {
  // Mirror the patch back into the engine's slide model so savePptx
  // emits the right bytes.
  const slide = deck.value?.opened?.deck.slides[activeIndex.value]
  if (!slide) return
  const el = slide.elements.find((e: any) => e.id === id) as any
  if (!el) return
  el.dirty = true
  if ('x' in patch || 'y' in patch || 'w' in patch || 'h' in patch || 'rotation' in patch) {
    el.dirtyTransform = true
    el.transform = el.transform || { offset: { x: 0, y: 0, cx: 0, cy: 0 }, rot: 0, flipH: false, flipV: false }
    const off = el.transform.offset || { x: 0, y: 0, cx: 0, cy: 0 }
    if (patch.x !== undefined) off.x = patch.x
    if (patch.y !== undefined) off.y = patch.y
    if (patch.w !== undefined) off.cx = patch.w
    if (patch.h !== undefined) off.cy = patch.h
    if (patch.rotation !== undefined) el.transform.rot = patch.rotation
    el.transform.offset = off
  }
  if ('text' in patch) {
    // Mutate the engine's text body so savePptx re-emits the right runs.
    if (el.text && el.text.paragraphs?.[0]) {
      el.text.paragraphs[0].runs = [{ text: patch.text ?? '' } as any]
      el.dirty = true
    }
  }
  if ('fill' in patch) {
    el.dirtyFill = true
    if (el.fill) (el.fill as any).color = (patch.fill ?? '').toUpperCase()
  }
  if ('stroke' in patch) {
    el.dirtyStroke = true
    if (el.stroke) (el.stroke as any).color = (patch.stroke ?? '').toUpperCase()
  }
}

const syncFromY = () => {
  if (!ydeck) return
  const remote = ydeck.toArray().map((m) => {
    const obj = m.toJSON() as any
    const shapesArr = (m.get('shapes') as Y.Array<unknown> | undefined)?.toArray?.() ?? []
    const shapes = shapesArr
      .map((s) => objToShape(((s as Y.Map<unknown>).toJSON?.() ?? s) as Record<string, unknown>))
      .filter((s) => s.id)
    const width = Number(obj.width ?? SLIDE_W_INCH * 914400)
    const height = Number(obj.height ?? SLIDE_H_INCH * 914400)
    const background = obj.background ? String(obj.background) : undefined
    const matched = localSlidesByIndex.value.find((s) => s?.index === Number(obj.index ?? 0))
    // Read remote notes text if present
    let remoteNotes = matched?.notes ?? ''
    const ynotes = obj.notes
    if (ynotes && typeof (ynotes as any).toString === 'function') {
      try { remoteNotes = (ynotes as any).toString() } catch { /* keep local */ }
    }
    return {
      index: Number(obj.index ?? 0),
      width,
      height,
      background,
      shapes,
      raw: (matched?.raw ?? null) as unknown as Slide,
      notes: remoteNotes,
    }
  })
  if (remote.length === 0) return
  slides.value = remote
  // Resolve picture dataURLs into HTMLImageElement for Konva.
  for (const slide of remote) {
    for (const shape of slide.shapes) {
      if (shape.type === 'picture' && shape.mediaData && !pictureImages[shape.id]) {
        const img = new window.Image()
        img.crossOrigin = 'anonymous'
        img.onload = () => {
          pictureImages[shape.id] = img
        }
        img.src = shape.mediaData
      }
    }
  }
  // v0.7.108 — propagate groupId changes to the engine so a remote peer's
  // group / ungroup also rebuilds slide.elements into a <p:grpSp>. Local
  // groupSelected / ungroupSelected already call the engine themselves and
  // mark the gid via markLocalGrouped / markLocalUngrouped, so the syncFromY
  // triggered by the local Yjs observe sees no diff.
  if (deck.value?.opened) {
    for (const slide of remote) {
      projectGroupsToEngine({
        shapes: slide.shapes,
        slideIdx: slide.index,
        opened: deck.value.opened,
        state: lastSyncedYjsGroupIdsBySlideIdx,
        engineMap: engineGroupIdByYjsGroupId,
      })
    }
  }
}

const seedYjs = () => {
  if (!ydeck) return
  const sourceShapeCount = slides.value.reduce((total, slide) => total + slide.shapes.length, 0)
  const currentSlides = ydeck.toArray()
  const currentShapeCount = currentSlides.reduce((total, slide) => {
    const shapes = slide.get('shapes') as Y.Array<unknown> | undefined
    return total + (shapes?.length ?? 0)
  }, 0)
  const sourceIds = new Set(slides.value.flatMap((slide) => slide.shapes.map((shape) => shape.id)))
  const currentIds = new Set(currentSlides.flatMap((slide) => {
    const shapes = slide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    return shapes?.toArray().map((shape) => String(shape.get('id') ?? '')) ?? []
  }))
  const hasMatchingShape = [...sourceIds].some((id) => currentIds.has(id))
  // The server PPTX is the source of truth on first open. A persisted Yjs
  // array from an earlier failed load can be non-empty but contain only blank
  // slides; letting it win makes a real presentation render as white canvas.
  // Preserve a non-empty collaborative deck, including intentionally blank
  // slides, and only replace the stale blank snapshot.
  const replaceStaleBlankSnapshot = sourceShapeCount > 0 && (!hasMatchingShape || currentShapeCount === 0)
  if (replaceStaleBlankSnapshot && ydeck.length > 0) {
    ydeck.delete(0, ydeck.length)
  }
  ydeck.doc?.transact(() => {
    if (ydeck!.length === 0) {
      for (const s of slides.value) {
        const yslide = new Y.Map<unknown>()
        yslide.set('index', s.index)
        yslide.set('width', s.width)
        yslide.set('height', s.height)
        yslide.set('background', s.background ?? '')
        const yshapes = new Y.Array<Y.Map<unknown>>()
        for (const sh of s.shapes) {
          const m = new Y.Map<unknown>()
          for (const [k, v] of Object.entries(shapeToObj(sh))) m.set(k, v)
          yshapes.push([m])
        }
        yslide.set('shapes', yshapes)
      // Per-slide Y.Text for speaker notes — collaborative edit on the
      // speaker notes textarea.
      const ynotes = new Y.Text()
      const noteText = slides.value[s.index]?.notes ?? ''
      if (noteText) ynotes.insert(0, noteText)
      yslide.set('notes', ynotes)
        ydeck!.push([yslide])
      }
    }
  })
}

// --- CRUD ---
const addShape = (type: PptxShape['type']) => {
  if (!ydeck) return
  ydeck.doc?.transact(() => {
    let yslide = ydeck!.get(activeIndex.value) as Y.Map<unknown> | undefined
    if (!yslide) return
    let yshapes = yslide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    if (!yshapes) {
      yshapes = new Y.Array<Y.Map<unknown>>()
      yslide.set('shapes', yshapes)
    }
    const created = addShapeOnDeck(deck.value as unknown as PptxShapeDeck, activeIndex.value, type, {
      x: 914400,
      y: 914400,
      cx: type === 'line' ? 1828800 : 1828800,
      cy: type === 'line' ? 0 : 914400,
    })
    if (!created) return
    const id = created.id
    const m = new Y.Map<unknown>()
    const base = { ...created, id, type }
    for (const [k, v] of Object.entries(base)) {
      if (v !== undefined) m.set(k, v as any)
    }
    yshapes.push([m])
    selectOnly(id)
    scheduleSave()
  })
}

const addSlide = () => {
  if (!ydeck || !deck.value?.opened) return
  ydeck.doc?.transact(() => {
    const sourceIndex = activeIndex.value
    const inserted = insertBlankSlideOnDeck(deck.value as unknown as PptxShapeDeck, sourceIndex)
    if (!inserted) return
    const newIndex = sourceIndex + 1
    const width = deck.value!.opened!.deck.size.cx
    const height = deck.value!.opened!.deck.size.cy
    const yslide = new Y.Map<unknown>()
    yslide.set('index', newIndex)
    yslide.set('width', width)
    yslide.set('height', height)
    yslide.set('background', '')
    yslide.set('shapes', new Y.Array<Y.Map<unknown>>())
    ydeck!.insert(newIndex, [yslide])
    slides.value.splice(newIndex, 0, { index: newIndex, width, height, shapes: [], raw: inserted })
    activeIndex.value = newIndex
    scheduleSave()
  })
}

const deleteSlide = (i: number) => {
  if (slides.value.length <= 1) return
  if (!ydeck || !deck.value?.opened) return
  ydeck.doc?.transact(() => {
    ydeck!.delete(i, 1)
    deck.value!.opened!.deck.slides.splice(i, 1)
    for (let j = 0; j < slides.value.length; j++) {
      if (j === i) continue
    }
    slides.value.splice(i, 1)
    if (activeIndex.value >= slides.value.length) activeIndex.value = slides.value.length - 1
    scheduleSave()
  })
}

const moveSlide = (from: number, to: number) => {
  if (to < 0 || to >= slides.value.length) return
  if (!ydeck || !deck.value?.opened) return
  ydeck.doc?.transact(() => {
    const arr = ydeck!.toArray()
    const [item] = arr.splice(from, 1)
    arr.splice(to, 0, item)
    ydeck!.delete(0, ydeck!.length)
    ydeck!.push(arr)
    const slideArr = deck.value!.opened!.deck.slides
    const [slideItem] = slideArr.splice(from, 1)
    slideArr.splice(to, 0, slideItem)
    slides.value = ydeck!.toArray().map((m) => {
      const obj = m.toJSON() as any
      return slides.value.find((s) => s.index === obj.index) ?? slides.value[0]
    })
    activeIndex.value = to
    scheduleSave()
  })
}

const deleteSelected = () => {
  if (!selectedId.value || !ydeck) return
  const id = selectedId.value
  ydeck.doc?.transact(() => {
    const yslide = ydeck!.get(activeIndex.value) as Y.Map<unknown> | undefined
    if (!yslide) return
    const yshapes = yslide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    if (!yshapes) return
    const i = yshapes.toArray().findIndex((m) => m.get('id') === id)
    if (i < 0) return
    yshapes.delete(i, 1)
    // Mirror: drop from engine slide too.
    const slide = deck.value?.opened?.deck.slides[activeIndex.value]
    if (slide) {
      const ei = slide.elements.findIndex((e: any) => e.id === id)
      if (ei >= 0) {
        slide.elements.splice(ei, 1)
        slide.structureDirty = true
      }
    }
    selectOnly(null)
    scheduleSave()
  })
}

// --- Save / load ---
const saveTimer = ref<number | null>(null)

// v0.7.139 — Clipboard + AI handlers (GenOffice pattern). Most delegate
// to existing engine actions; AI panel reuses the existing
// CollabAiPolishDialog. canPaste tracks whether the local clipboard has
// a copied item (set by onCopy/onCut).
const canPaste = ref(false)
const onCopySelected = () => {
  if (!selectedId.value || !ydeck) return
  try {
    // Copy via the document.execCommand fallback so external paste works.
    const node = document.querySelector(`[data-shape-id="${selectedId.value}"]`)
    if (!node) return
    const range = document.createRange()
    range.selectNodeContents(node)
    const sel = window.getSelection()
    sel?.removeAllRanges()
    sel?.addRange(range)
    document.execCommand('copy')
    sel?.removeAllRanges()
    canPaste.value = true
  } catch {}
}
const onCutSelected = () => {
  if (!selectedId.value) return
  onCopySelected()
  // Remove the shape after cutting
  if (!ydeck) return
  ydeck.doc?.transact(() => {
    const yslide = ydeck!.get(activeIndex.value) as Y.Map<unknown> | undefined
    if (!yslide) return
    const yshapes = yslide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    if (!yshapes) return
    const arr = yshapes.toArray()
    const i = arr.findIndex((m) => m.get('id') === selectedId.value)
    if (i < 0) return
    yshapes.delete(i, 1)
  })
}
const onPasteSelected = () => {
  if (!canPaste.value) return
  try {
    const pasted = document.execCommand('paste')
    if (!pasted) return
    // The browser pasted HTML at caret; treat the inserted node as a duplicate.
    canPaste.value = false
  } catch {}
}
// v0.7.139 — AI handlers. The full AI dialog lives in CollabAiPolishDialog
// (already wired in this view via the brandmark / sidekick). These handlers
// open that dialog with a sensible preset action.
const onOpenAiPanel = () => {
  document.dispatchEvent(new CustomEvent('wk-slide-ai-open'))
}
const onAiPolishSelected = () => {
  document.dispatchEvent(new CustomEvent('wk-slide-ai-action', { detail: { action: 'polish' } }))
}
const onAiSuggestSlide = () => {
  document.dispatchEvent(new CustomEvent('wk-slide-ai-action', { detail: { action: 'suggest' } }))
}
const onAiRewrite = () => {
  document.dispatchEvent(new CustomEvent('wk-slide-ai-action', { detail: { action: 'rewrite' } }))
}
const onAiImage = () => {
  document.dispatchEvent(new CustomEvent('wk-slide-ai-action', { detail: { action: 'image' } }))
}
const onFormatPainter = () => {
  MessagePlugin.info('格式刷：复制当前选中形状的格式，再次点击形状即可应用。')
}

// v0.7.139 — Font group state and helpers.
const fontPickerOpen = ref(false)
const fontSizePickerOpen = ref(false)
const FONT_FAMILIES = [
  'MiSans', 'Oranienbaum', 'Liter', 'Calibri', 'PingFang SC', 'Microsoft YaHei',
  'Source Han Sans CN', 'Noto Sans CJK SC', 'Arial', 'sans-serif',
]
const selectedFontLabel = computed(() => selectedShape.value?.fontFamily || '字体')
const bumpFontSize = (delta: number) => {
  if (!selectedId.value) return
  const cur = Number(selectedShape.value?.fontSize ?? 18)
  updateShape(selectedId.value, { fontSize: Math.max(8, Math.min(96, cur + delta)) })
}

// v0.7.139 — Paragraph group helpers.
const alignText = (align: 'left' | 'center' | 'right' | 'justify') => {
  if (!selectedId.value) return
  updateShape(selectedId.value, { textAlign: align })
}
const toggleBullets = () => {
  if (!selectedId.value) return
  const cur = selectedShape.value?.bulletList ?? false
  updateShape(selectedId.value, { bulletList: !cur })
}
const toggleNumbered = () => {
  if (!selectedId.value) return
  const cur = selectedShape.value?.numberedList ?? false
  updateShape(selectedId.value, { numberedList: !cur })
}
const bumpIndent = (delta: number) => {
  if (!selectedId.value) return
  const cur = Number(selectedShape.value?.indent ?? 0)
  updateShape(selectedId.value, { indent: Math.max(0, Math.min(8, cur + delta)) })
}

const duplicateSelected = () => {
  if (!selectedId.value || !ydeck) return
  const id = selectedId.value
  ydeck.doc?.transact(() => {
    const yslide = ydeck!.get(activeIndex.value) as Y.Map<unknown> | undefined
    if (!yslide) return
    const yshapes = yslide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    if (!yshapes) return
    const arr = yshapes.toArray()
    const i = arr.findIndex((m) => m.get('id') === id)
    if (i < 0) return
    const src = arr[i]
    const newId = 'shape-' + Date.now() + '-' + Math.floor(Math.random() * 1000)
    const copy = new Y.Map<unknown>()
    for (const [k, v] of Object.entries(src.toJSON())) copy.set(k, v)
    copy.set('id', newId)
    copy.set('x', Number(src.get('x') ?? 0) + 914400 / 4) // +0.25 inch offset
    copy.set('y', Number(src.get('y') ?? 0) + 914400 / 4)
    yshapes.insert(i + 1, [copy])
    // Mirror engine
    const slide = deck.value?.opened?.deck.slides[activeIndex.value]
    if (slide) {
      const ei = slide.elements.findIndex((e: any) => e.id === id)
      const srcEl = ei >= 0 ? slide.elements[ei] : null
      const newEl = srcEl ? JSON.parse(JSON.stringify(srcEl)) : { id: newId, type: 'shape' }
      newEl.id = newId
      const o = (newEl.transform && newEl.transform.offset) || { x: 0, y: 0, cx: 0, cy: 0 }
      o.x = (o.x || 0) + 914400 / 4
      o.y = (o.y || 0) + 914400 / 4
      newEl.transform = { offset: o, rot: 0, flipH: false, flipV: false }
      newEl.dirty = true
      slide.elements.splice(ei + 1, 0, newEl)
      slide.structureDirty = true
    }
    selectOnly(newId)
    scheduleSave()
  })
}

const reorderSelected = (dir: 1 | -1) => {
  if (!selectedId.value || !ydeck) return
  const id = selectedId.value
  ydeck.doc?.transact(() => {
    const yslide = ydeck!.get(activeIndex.value) as Y.Map<unknown> | undefined
    if (!yslide) return
    const yshapes = yslide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    if (!yshapes) return
    const arr = yshapes.toArray()
    const i = arr.findIndex((m) => m.get('id') === id)
    const target = i + dir
    if (i < 0 || target < 0 || target >= arr.length) return
    yshapes.delete(i, 1)
    yshapes.insert(target, [arr[i]])
    const slide = deck.value?.opened?.deck.slides[activeIndex.value]
    if (slide) {
      const ei = slide.elements.findIndex((e: any) => e.id === id)
      if (ei >= 0) {
        const el = slide.elements.splice(ei, 1)[0]
        slide.elements.splice(ei + dir, 0, el)
        slide.structureDirty = true
      }
    }
    scheduleSave()
  })
}
const bringForward = () => reorderSelected(1)
const sendBackward = () => reorderSelected(-1)

const bringToFront = () => {
  if (!selectedId.value || !ydeck) return
  const id = selectedId.value
  ydeck.doc?.transact(() => {
    const yslide = ydeck!.get(activeIndex.value) as Y.Map<unknown> | undefined
    if (!yslide) return
    const yshapes = yslide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    if (!yshapes) return
    const arr = yshapes.toArray()
    const i = arr.findIndex((m) => m.get('id') === id)
    if (i < 0 || i === arr.length - 1) return
    yshapes.delete(i, 1)
    yshapes.insert(arr.length - 1, [arr[i]])
    const slide = deck.value?.opened?.deck.slides[activeIndex.value]
    if (slide) {
      const ei = slide.elements.findIndex((e: any) => e.id === id)
      if (ei >= 0 && ei < slide.elements.length - 1) {
        const el = slide.elements.splice(ei, 1)[0]
        slide.elements.push(el)
        slide.structureDirty = true
      }
    }
    scheduleSave()
  })
}

// v0.7.79 — PPT shape rotation. Rotates the selected shape by ±90°.
const rotateSelected = (delta: number) => {
  if (!deck.value || selectedId.value == null) return
  const slide = deck.value.opened?.deck.slides[activeIndex.value]
  if (!slide) return
  const el = slide.elements.find((e: any) => e.id === selectedId.value) as any
  if (!el) return
  const current = Number(el.transform?.rot ?? 0)
  const direction = delta > 0 ? 1 : -1
  const next = stepRotation90(current, direction)
  updateShape(selectedId.value, { rotation: next } as any)
}

const sendToBack = () => {
  if (!selectedId.value || !ydeck) return
  const id = selectedId.value
  ydeck.doc?.transact(() => {
    const yslide = ydeck!.get(activeIndex.value) as Y.Map<unknown> | undefined
    if (!yslide) return
    const yshapes = yslide.get('shapes') as Y.Array<Y.Map<unknown>> | undefined
    if (!yshapes) return
    const arr = yshapes.toArray()
    const i = arr.findIndex((m) => m.get('id') === id)
    if (i <= 0) return
    yshapes.delete(i, 1)
    yshapes.insert(0, [arr[i]])
    const slide = deck.value?.opened?.deck.slides[activeIndex.value]
    if (slide) {
      const ei = slide.elements.findIndex((e: any) => e.id === id)
      if (ei > 0) {
        const el = slide.elements.splice(ei, 1)[0]
        slide.elements.unshift(el)
        slide.structureDirty = true
      }
    }
    scheduleSave()
  })
}

const scheduleSave = () => {
  savetagClass.dirty = true
  saveLabel.value = '编辑中…'
  if (saveTimer.value) window.clearTimeout(saveTimer.value)
  saveTimer.value = window.setTimeout(() => onForceSave(), 1500)
}

const transitionInput = ref<SlideTransitionKind>('none')
const onTransitionCommit = () => {
  if (!deck.value) return
  setSlideTransitionOnDeck(deck.value as unknown as PptxShapeDeck, activeIndex.value, transitionInput.value)
  scheduleSave()
}
const loadTransitionForActive = () => {
  if (!deck.value) return
  transitionInput.value = getSlideTransitionOnDeck(deck.value as unknown as PptxShapeDeck, activeIndex.value)
}
// v0.7.61 — PPT comment round-trip: cache backend comments + write to .pptx on save.
let cachedSlideComments: CollabDocComment[] = []
const onSlideCommentsLoaded = (comments: CollabDocComment[]) => {
  cachedSlideComments = comments
}

const writeSlideCommentsToArchive = (opened: any) => {
  if (cachedSlideComments.length === 0) return
  // Group by slide index from anchor_ref JSON.
  const bySlide = new Map<number, CollabDocComment[]>()
  for (const c of cachedSlideComments) {
    if (c.anchor_type !== 'slide') continue
    let slideIdx = -1
    try {
      const o = JSON.parse(c.anchor_ref || '{}')
      slideIdx = typeof o.slide === 'number' ? o.slide : -1
    } catch {}
    if (slideIdx < 0) continue
    const arr = bySlide.get(slideIdx) || []
    arr.push(c)
    bySlide.set(slideIdx, arr)
  }
  for (const [slideIdx, comments] of bySlide) {
    const slide = opened.deck.slides[slideIdx]
    if (!slide) continue
    // Skip comments already present in the archive (by author+text) to avoid duplicates.
    const existing = getSlideComments(opened, slide.path)
    const existingKeys = new Set(existing.map((c: any) => `${c.author}|${c.text}`))
    for (const c of comments) {
      const key = `${c.author_name || ''}|${c.body || ''}`
      if (existingKeys.has(key)) continue
      addSlideComment(opened, slideIdx, {
        author: c.author_name || 'Unknown',
        text: c.body || '',
      })
    }
  }
}

const onForceSave = async () => {
  if (!deck.value) return
  savetagClass.saving = true
  saveLabel.value = '保存中…'
  try {
    if (deck.value.opened) writeSlideCommentsToArchive(deck.value.opened)
    const bytes = await savePptxShapeBytes(deck.value as unknown as PptxShapeDeck)
    await uploadCollabDocBytes(props.docId, bytes, `${props.title || 'collab-doc'}.pptx`)
    savetagClass.dirty = false
    saveLabel.value = '已保存'
    saveError.value = null
    setTimeout(() => { if (saveLabel.value === '已保存') saveLabel.value = '未修改' }, 1500)
  } catch (e: any) {
    saveError.value = e?.message || String(e)
    saveLabel.value = '保存失败'
  } finally {
    savetagClass.saving = false
  }
}

const onDownload = async () => {
  downloading.value = true
  try {
    const bytes = deck.value ? await savePptxShapeBytes(deck.value as unknown as PptxShapeDeck) : null
    if (bytes) {
      const ab = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer
      const blob = new Blob([ab], {
        type: 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
      })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${props.title || 'collab-doc'}.pptx`
      a.click()
      URL.revokeObjectURL(url)
    } else {
      await downloadCollabDocBytes(props.docId)
    }
  } catch (e: any) {
    MessagePlugin.error(`下载失败: ${e?.message ?? e}`)
  } finally {
    downloading.value = false
  }
}

// Local bridge so the inline <CollabSlideThemePanel/> below can emit
// directly into the existing wk-slide-theme-apply window listener.
const onThemePanelApply = (preset: SlideThemePreset) => {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent('wk-slide-theme-apply', { detail: preset }))
}

// Slide theme persistence (v0.7.95): listen to wk-slide-theme-apply from
// CollabSlideThemePanel. Rewrite theme*.xml in every theme part, remap explicit
// srgbClr, mark dirty and trigger auto-save so the new palette round-trips
// back to collab-doc storage.
const onSlideThemeApply = async (e: Event) => {
  const preset = (e as CustomEvent<SlideThemePreset>).detail
  if (!preset || !deck.value || !deck.value.opened) return
  const spec = {
    name: preset.id,
    colors: preset.colors,
    ...(preset.majorFont ? { majorFont: preset.majorFont } : {}),
    ...(preset.minorFont ? { minorFont: preset.minorFont } : {}),
  }
  const themePatched = applyThemeToDeck(
    deck.value as unknown as PptxShapeDeck,
    spec,
  )
  const remapped = recolorDeck(
    deck.value as unknown as PptxShapeDeck,
    spec,
  )
  // Force re-render: rebuild slides array reference so Vue-Konva picks up the
  // new fill / stroke values
  if (deck.value.slides.length > 0) {
    slides.value = [...deck.value.slides]
  }
  if (themePatched > 0 || remapped > 0) {
    savetagClass.dirty = true
    saveLabel.value = '主题已应用 · 待保存'
    MessagePlugin.success(`主题 ${preset.name} 已应用: ${themePatched} theme*, ${remapped} srgbClr 重映射`)
    scheduleSave()
  } else {
    MessagePlugin.warning(`主题 ${preset.name} 未匹配到任何 theme*.xml 或 srgbClr`)
  }
}

// --- v0.7.97 — slide layout switcher (master / layout binding) ---
const sizeW = computed(() => deck.value?.slides[0]?.width ?? SLIDE_W_INCH * 914400)
const sizeH = computed(() => deck.value?.slides[0]?.height ?? SLIDE_H_INCH * 914400)
const availableLayouts = computed(() => {
  if (!deck.value) return []
  return listSlideLayouts(deck.value as unknown as PptxShapeDeck)
})
const missingBuiltins = computed(() => {
  const present = new Set(availableLayouts.value.map((l) => l.name))
  const catalog = [
    { key: 'titleSlide', name: 'Title Slide' },
    { key: 'titleContent', name: 'Title and Content' },
    { key: 'sectionHeader', name: 'Section Header' },
    { key: 'twoContent', name: 'Two Content' },
    { key: 'titleOnly', name: 'Title Only' },
    { key: 'blank', name: 'Blank' },
  ]
  return catalog.filter((b) => !present.has(b.name))
})
/* onLayoutSelect removed — replaced by applyLayout (popover-driven) */

// --- v0.7.96 — fullscreen present mode ---
const presentIndex = ref(0)
const presentSlide = computed(() => slides.value[presentIndex.value] ?? null)
const presentShapes = computed(() => presentSlide.value?.shapes ?? [])
const nextSlide = computed(() => slides.value[presentIndex.value + 1] ?? null)

const onEnterPresent = () => {
  if (!slides.value.length || loading.value) return
  presentIndex.value = activeIndex.value
  presentMode.value = true
}
const onExitPresent = () => {
  presentMode.value = false
  // sync back so editor resumes on the slide the presenter was last showing
  activeIndex.value = presentIndex.value
}
const presentPrev = () => {
  if (presentIndex.value > 0) presentIndex.value -= 1
}
const presentNext = () => {
  if (presentIndex.value < slides.value.length - 1) presentIndex.value += 1
}
const onPresentKeydown = (e: KeyboardEvent) => {
  if (!presentMode.value) return
  if (e.key === 'Escape') { e.preventDefault(); onExitPresent(); return }
  if (e.key === 'ArrowRight' || e.key === 'PageDown' || e.key === ' ') {
    e.preventDefault()
    if (e.shiftKey && e.key === 'ArrowRight') { presentPrev(); return }
    presentNext()
    return
  }
  if (e.key === 'ArrowLeft' || e.key === 'PageUp') {
    e.preventDefault()
    presentPrev()
    return
  }
  if (e.key === 'Home') { e.preventDefault(); presentIndex.value = 0; return }
  if (e.key === 'End') { e.preventDefault(); presentIndex.value = slides.value.length - 1; return }
}

if (typeof window !== 'undefined') {
  window.addEventListener('wk-slide-theme-apply', onSlideThemeApply as EventListener)
  window.addEventListener('keydown', onPresentKeydown)
}

const triggerUpload = () => fileInput.value?.click()

const onUploadFile = async (e: Event) => {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  uploading.value = true
  try {
    const bytes = new Uint8Array(await file.arrayBuffer())
    if (!await onLoadBytes(bytes)) {
      throw new Error(error.value || '无法解析 PPTX 文件')
    }
    // Push to server immediately so a tab refresh sees the same content.
    await uploadCollabDocBytes(props.docId, bytes, file.name)
  } catch (e: any) {
    saveError.value = e?.message || String(e)
  } finally {
    uploading.value = false
    if (fileInput.value) fileInput.value.value = ''
  }
}

const onLoadBytes = async (bytes: Uint8Array): Promise<boolean> => {
  loading.value = true
  try {
    const fresh = await openPptxShapes(bytes)
    deck.value = fresh
    slides.value = fresh.slides
    activeIndex.value = 0
    error.value = null
    recoveryMessage.value = null
    seedYjs()
    syncFromY()
    loading.value = false
    return true
  } catch (e: any) {
    error.value = e?.message || String(e)
    loading.value = false
    return false
  }
}

const getCollabDocRequestHeaders = () => {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${props.token}`,
  }
  const tenantId = localStorage.getItem('weknora_selected_tenant_id')
  if (tenantId) headers['X-Tenant-ID'] = tenantId
  return headers
}

const initializeBlankDeck = async (message?: string) => {
  const fresh = await newPptxShapeDeck()
  deck.value = fresh
  slides.value = fresh.slides
  activeIndex.value = 0
  error.value = null
  recoveryMessage.value = message ?? null
  loading.value = false
  seedYjs()
  syncFromY()
}

// --- Lifecycle ---
handle = useYjsCollabDoc({ docId: props.docId, token: props.token, displayName: props.displayName, tenantId: props.tenantId })
ydoc = handle.ydoc
ydeck = handle.ydoc.getArray<Y.Map<unknown>>('slide:deck')
connected.value = false
handle.provider.on('status', (event: any) => {
  connected.value = event.status === 'connected'
})
handle.provider.awareness.on('change', () => {
  const out: Array<{ clientId: number; displayName: string; color: string }> = []
  const cursors: Array<{ clientId: number; x?: number; y?: number; color: string; name: string }> = []
  const selections: Array<{ clientId: number; shapeId: string; color: string; name: string }> = []
  handle!.provider.awareness.getStates().forEach((state: any, clientId: number) => {
    if (clientId === handle!.ydoc.clientID) return
    const u = state.user || {}
    out.push({ clientId, displayName: u.name || '匿名用户', color: u.color || '#58a6ff' })
    const cur = state.cursor
    if (cur && cur.slide === activeIndex.value) {
      cursors.push({
        clientId,
        x: cur.x,
        y: cur.y,
        color: u.color || '#58a6ff',
        name: u.name || '匿名用户',
      })
    }
    const sel = state.selection
    if (sel && sel.slide === activeIndex.value) {
      const ids = Array.isArray(sel.shapeIds) && sel.shapeIds.length ? sel.shapeIds : (sel.shapeId ? [sel.shapeId] : [])
      for (const shapeId of ids) {
        selections.push({
          clientId,
          shapeId,
          color: u.color || '#58a6ff',
          name: u.name || '匿名用户',
        })
      }
    }
  })
  peers.value = out
  remoteCursors.value = cursors
  remoteSelections.value = selections
})

// Try to fetch existing pptx from server, else build a fresh one.
;(async () => {
  try {
    const existing = await fetch(`/api/v1/collaborative-docs/${encodeURIComponent(props.docId)}/download`, {
      headers: getCollabDocRequestHeaders(),
    })
    if (existing.ok) {
      const buf = new Uint8Array(await existing.arrayBuffer())
      if (buf.byteLength > 100) {
        if (await onLoadBytes(buf)) return
        await initializeBlankDeck('原演示文稿无法解析，已恢复为新的空白演示文稿；首次编辑后会保存为有效 PPTX。')
        return
      }
    } else if (existing.status !== 404) {
      throw new Error(`加载演示文稿失败（HTTP ${existing.status}），请检查登录状态和当前工作空间权限。`)
    }
  } catch (e: any) {
    error.value = e?.message || '加载演示文稿失败'
    loading.value = false
    return
  }
  // No existing pptx — start with engine blank.
  await initializeBlankDeck()
})()

if (ydoc) {
  undoManagerRef.value = new Y.UndoManager(ydeck)
  undoManagerRef.value.on('stack-item-added', () => {
    canUndo.value = undoManagerRef.value.undoStack.length > 0
    canRedo.value = undoManagerRef.value.redoStack.length > 0
  })
  undoManagerRef.value.on('stack-item-popped', () => {
    canUndo.value = undoManagerRef.value.undoStack.length > 0
    canRedo.value = undoManagerRef.value.redoStack.length > 0
  })
}
ydeck.observeDeep(() => syncFromY())
if (typeof window !== 'undefined') window.addEventListener('keydown', onKeydown)

onBeforeUnmount(() => {
  if (saveTimer.value) window.clearTimeout(saveTimer.value)
  if (typeof window !== 'undefined') {
    window.removeEventListener('keydown', onKeydown)
    window.removeEventListener('keydown', onPresentKeydown)
    window.removeEventListener('wk-slide-theme-apply', onSlideThemeApply as EventListener)
  }
  handle?.destroy()
})
</script>

<style scoped>
.slide-present-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.96);
  z-index: 9999;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
}
.slide-present-shell {
  position: relative;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
}
.slide-present-stage {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  min-height: 0;
}
.slide-present-svg {
  max-width: 100%;
  max-height: 100%;
  width: auto;
  height: auto;
  background: #ffffff;
  box-shadow: 0 30px 60px rgba(0, 0, 0, 0.45);
}
.slide-present-controls {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 24px;
  background: rgba(15, 23, 42, 0.85);
  border-radius: 999px;
  color: #f1f5f9;
  font-size: 14px;
}
.slide-present-btn {
  background: rgba(255, 255, 255, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.2);
  color: #f8fafc;
  padding: 6px 14px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}
.slide-present-btn:disabled { opacity: 0.4; cursor: not-allowed; }
.slide-present-btn--exit { background: rgba(220, 38, 38, 0.2); border-color: rgba(220, 38, 38, 0.4); }
.slide-present-divider { width: 1px; height: 20px; background: rgba(255, 255, 255, 0.2); }
.slide-present-counter { font-variant-numeric: tabular-nums; min-width: 60px; text-align: center; }
/* v0.7.74 — PPT bottom status bar */
.collab-slide-konva__statusbar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 14px;
  background: var(--slide-chrome, #161a22);
  border-top: 1px solid var(--slide-chrome-border, #2c313b);
  color: var(--slide-chrome-text, #9ca6b4);
  font-size: 11px;
  user-select: none;
  height: 26px;
  flex: none;
}
.collab-slide-konva__statusbar-item { white-space: nowrap; }
.collab-slide-konva__statusbar-sep { opacity: 0.4; }
.collab-slide-konva__statusbar-spacer { flex: 1; }
.collab-slide-konva__statusbar-btn {
  border: 0;
  background: transparent;
  color: var(--slide-chrome-text, #9ca6b4);
  cursor: pointer;
  font-size: 14px;
  padding: 0 6px;
  line-height: 1;
}
.collab-slide-konva__statusbar-btn:hover { color: var(--td-brand-color, #5aa8ff); }
.collab-slide-konva__statusbar-zoom { min-width: 42px; text-align: center; font-variant-numeric: tabular-nums; }

/* v0.7.115 — Ribbon groups now use the shared .ribbon-group rule (GenOffice
 * styles.css), so we only ship editor-specific tweaks here: hint text, the
 * layout-picker dropdown, and theme panel. The previous .collab-slide-konva__tool-btn
 * overrides are gone because nothing in the template uses those classes
 * anymore — buttons are styled by the shared .rb-big / .rb-small rules. */
.collab-slide-konva__ribbon-groups { display: flex; align-items: stretch; min-height: 80px; }
.collab-slide-konva__tool-group { padding: 2px 4px; }
.collab-slide-konva__tool-group:first-of-type { padding-left: 4px; }
.collab-slide-konva__ribbon-hint { align-self: center; padding: 0 18px; color: #687385; font-size: 11px; white-space: nowrap; }
/* Layout picker rides on the shared .rb-big button: an icon row + label on
 * top, with a select chip tucked under it (replaces the old stand-alone label
 * dropdown). */
/* ===== Layout dropdown (GenOffice-style popover replacing native <select>) =====
   The button looks like a normal .rb-big with a chevron; clicking toggles a
   popover menu that lists layouts. Click-outside dismisses it. */
.collab-slide-konva__layout-btn { min-width: 92px; }
.collab-slide-konva__layout-btn.is-open .rb-big-icon { background: var(--rb-pressed, rgba(24,90,189,0.16)); }
.collab-slide-konva__layout-menu {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  z-index: 80;
  min-width: 240px;
  max-height: 320px;
  overflow-y: auto;
  padding: 6px;
  background: var(--app-surface-raised, #fff);
  border: 1px solid var(--app-border-strong, #cbd0d7);
  border-radius: 6px;
  box-shadow: 0 8px 24px rgba(0,0,0,0.16);
  font-size: 12px;
  color: var(--app-text, #1f232b);
}
.collab-slide-konva__layout-menu-title { font-size: 11px; color: var(--app-text-muted, #5a6473); padding: 4px 8px 6px; letter-spacing: 0.04em; text-transform: uppercase; }
.collab-slide-konva__layout-menu-item {
  display: flex; flex-direction: column; gap: 1px; width: 100%;
  padding: 6px 10px; border: 0; border-radius: 4px;
  background: transparent; color: inherit; cursor: pointer; text-align: left;
}
.collab-slide-konva__layout-menu-item:hover { background: var(--app-surface, #f1f5fb); }
.collab-slide-konva__layout-menu-name { font-size: 13px; font-weight: 500; }
.collab-slide-konva__layout-menu-meta { font-size: 11px; color: var(--app-text-muted, #5a6473); }
.collab-slide-konva__layout-menu-sep { margin: 6px 6px 2px; padding: 0 4px; font-size: 11px; color: var(--app-text-muted, #5a6473); border-top: 1px solid var(--app-border, #e1e4e8); padding-top: 6px; }
.collab-slide-konva__layout-menu-empty { padding: 14px; text-align: center; color: var(--app-text-muted, #5a6473); }
/* The ribbon group that hosts the layout button needs to anchor the popover
   without clipping — overflow on the panel is auto, not hidden. */
.collab-slide-konva__layout-dropdown {
  margin: 0 -4px -2px;
  padding: 1px 18px 1px 6px;
  border: 1px solid var(--rb-border);
  border-radius: 3px;
  outline: none;
  background: #fff;
  color: var(--rb-text);
  font-size: 11px;
  font-family: inherit;
  max-width: 132px;
  text-overflow: ellipsis;
}
.collab-slide-konva__layout-dropdown:focus { border-color: #185abd; box-shadow: 0 0 0 2px rgba(24,90,189,.12); }

.slide-present-notes {
  position: absolute;
  bottom: 100px;
  right: 24px;
  max-width: 360px;
  padding: 14px 18px;
  background: rgba(15, 23, 42, 0.92);
  color: #f1f5f9;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.15);
}
.slide-present-notes-label { font-size: 11px; color: #cbd5e1; margin-bottom: 6px; letter-spacing: 0.05em; }
.slide-present-notes-body { font-size: 14px; line-height: 1.5; white-space: pre-wrap; }
.slide-present-next-preview {
  position: absolute;
  bottom: 100px;
  left: 24px;
  max-width: 240px;
  padding: 12px 16px;
  background: rgba(15, 23, 42, 0.85);
  color: #e2e8f0;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.15);
}
.slide-present-next-label { font-size: 11px; color: #94a3b8; margin-bottom: 4px; letter-spacing: 0.05em; }
.slide-present-next-title { font-size: 14px; }
.collab-slide-konva { display: flex; flex-direction: column; height: 100%; min-height: 0; background: var(--td-bg-color-container); overflow: hidden; }
.collab-slide-konva__toolbar { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid var(--td-component-stroke); flex-wrap: wrap; flex: 0 0 auto; }
.collab-slide-konva__title { font-weight: 600; }
.collab-slide-konva__kind { font-size: 11px; padding: 2px 8px; border-radius: 999px; background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-slide-konva__connection, .collab-slide-konva__savetag { font-size: 12px; padding: 2px 8px; border-radius: 999px; }
.collab-slide-konva__connection { background: var(--td-bg-color-secondarycontainer); }
.collab-slide-konva__connection.connected { background: var(--td-success-color-1); color: var(--td-success-color-7); }
.collab-slide-konva__savetag.dirty { background: var(--td-warning-color-1); color: var(--td-warning-color-7); }
.collab-slide-konva__savetag.saving { background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-slide-konva__peers { display: flex; gap: 4px; margin-left: auto; }
.collab-slide-konva__peer { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; color: white; font-size: 11px; font-weight: 600; }
.collab-slide-konva__body { flex: 1; display: flex; min-height: 0; }
.collab-slide-konva__thumbs { width: 180px; overflow-y: auto; padding: 8px; border-right: 1px solid var(--td-component-stroke); display: flex; flex-direction: column; gap: 8px; }
.collab-slide-konva__thumb { padding: 8px 10px; border: 1px solid var(--td-component-stroke); border-radius: 4px; background: var(--td-bg-color-container); cursor: pointer; text-align: left; display: flex; align-items: center; gap: 4px; flex-wrap: wrap; }
.collab-slide-konva__thumb.active { border-color: var(--td-brand-color-7); background: var(--td-brand-color-1); }
.collab-slide-konva__thumb-num { font-size: 11px; color: var(--td-text-color-secondary); }
.collab-slide-konva__thumb-title { flex: 1; font-size: 12px; }
.collab-slide-konva__iconbtn { border: none; background: transparent; cursor: pointer; font-size: 11px; padding: 0 4px; }
.collab-slide-konva__iconbtn.danger { color: var(--td-error-color-7); }
.collab-slide-konva__iconbtn:disabled { opacity: 0.4; cursor: not-allowed; }
.collab-slide-konva__stage-wrap { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 24px; overflow: auto; background: var(--slide-chrome-raised, var(--app-surface-bg, #f3f4f6)); }
.collab-slide-konva__zoom-info { font-size: 11px; color: var(--td-text-color-secondary); margin-bottom: 8px; }
.collab-slide-konva__stage { background: white; box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08); }
.collab-slide-konva__inspector { width: 240px; padding: 12px; border-left: 1px solid var(--td-component-stroke); overflow-y: auto; }
.collab-slide-konva__inspector h3 { font-size: 13px; margin: 0 0 12px 0; }
.collab-slide-konva__inspector label { display: block; font-size: 11px; margin-bottom: 8px; color: var(--td-text-color-secondary); }
.collab-slide-konva__inspector input, .collab-slide-konva__inspector textarea { width: 100%; font-size: 12px; padding: 4px 6px; border: 1px solid var(--td-component-stroke); border-radius: 4px; margin-top: 4px; }
.collab-slide-konva__error { color: var(--td-error-color-7); padding: 8px 12px; }
.collab-slide-konva__recovery { color: var(--td-warning-color-7); padding: 8px 12px; margin: 0; }
.collab-slide-konva__recovery { color: var(--td-warning-color-7); padding: 8px 12px; margin: 0; }
.collab-slide-konva__loading { padding: 24px; }

.collab-slide-konva__notes {
  border-top: 1px solid var(--td-component-stroke);
  padding: 12px 16px;
  background: var(--td-bg-color-container);
  flex: 0 0 auto;
}
.collab-slide-konva__notes-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 6px;
}
.collab-slide-konva__notes-status {
  font-size: 11px;
  font-weight: 400;
  color: var(--td-text-color-secondary);
}
.collab-slide-konva__notes-textarea {
  width: 100%;
  box-sizing: border-box;
  padding: 8px 10px;
  font-family: inherit;
  font-size: 13px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 4px;
  resize: vertical;
}
.collab-slide-konva__notes-hint {
  font-size: 11px;
  color: var(--td-text-color-secondary);
  margin: 4px 0 0 0;
}
.collab-slide-konva__divider {
  display: inline-block;
  width: 1px;
  height: 18px;
  background: var(--td-component-stroke);
  margin: 0 4px;
  vertical-align: middle;
}

.collab-slide-konva__modal-bg {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
}
.collab-slide-konva__modal {
  background: var(--td-bg-color-container);
  padding: 20px 24px;
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15);
  min-width: 280px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.collab-slide-konva__modal h3 {
  margin: 0 0 4px 0;
  font-size: 16px;
}
.collab-slide-konva__modal label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}
.collab-slide-konva__modal input[type=number] {
  width: 80px;
  padding: 4px 8px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 4px;
}
.collab-slide-konva__modal-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  margin-top: 8px;
}

/* GenOffice-inspired web workbench: a calm chrome frame around the canvas,
   with dense controls only where they are useful. v0.7.198 — defaults follow
   global --app-* tokens, so chrome aligns with global theme-mode (light/dark). */
.collab-slide-konva {
  --slide-chrome: var(--app-surface-raised, #ffffff);
  --slide-chrome-raised: var(--app-surface-bg, #f3f4f6);
  --slide-chrome-border: var(--app-border, #e5e7eb);
  --slide-chrome-muted: var(--app-text-muted, #6b7280);
  --slide-accent: #185abd;
  min-height: 0;
  height: 100%;
  background: var(--app-page-bg, #f3f4f6);
  color: var(--app-text, #1f232b);
  overflow: hidden;
}
.collab-slide-konva__titlebar {
  min-height: 48px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 14px;
  background: var(--slide-chrome);
  border-bottom: 1px solid var(--slide-chrome-border);
  flex: 0 0 auto;
}
.collab-slide-konva__brandmark {
  width: 26px;
  height: 26px;
  display: grid;
  place-items: center;
  flex: none;
  border-radius: 6px;
  background: #185abd;
  color: white;
  font: 800 15px/1 Georgia, serif;
  box-shadow: none;
}
.collab-slide-konva__file-meta { min-width: 0; }
.collab-slide-konva__title { max-width: 330px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #1f232b; font-size: 14px; font-weight: 650; }
.collab-slide-konva__file-subtitle { display: flex; align-items: center; gap: 7px; margin-top: 3px; color: var(--slide-chrome-muted); font-size: 10px; }
.collab-slide-konva__file-dot { color: #556070; }
.collab-slide-konva__title-actions { display: flex; align-items: center; gap: 7px; margin-left: auto; }
/* Title-bar chips: visible on the light surface (the title bar is now #ffffff).
#   The dark-mode color rules above use light-green ink on dark backgrounds,
#   which is unreadable on the light titlebar — replace with a subtle light
#   chip that follows the same tone but darker text. */
.collab-slide-konva__connection, .collab-slide-konva__savetag { display: inline-flex; align-items: center; gap: 6px; padding: 5px 9px; border: 1px solid var(--app-border, #e1e4e8); border-radius: 6px; background: var(--app-surface-raised, #fff); color: var(--app-text-muted, #5a6473); font-size: 11px; font-weight: 500; }
.collab-slide-konva__connection i { width: 7px; height: 7px; display: inline-block; border-radius: 50%; background: #94a3b8; }
.collab-slide-konva__connection.connected { border-color: rgba(22,163,74,0.32); color: #15803d; background: rgba(22,163,74,0.08); }
.collab-slide-konva__connection.connected i { background: #16a34a; box-shadow: 0 0 0 3px rgba(22,163,74,0.18); }
.collab-slide-konva__savetag.dirty { border-color: rgba(217,119,6,0.32); color: #b45309; background: rgba(217,119,6,0.08); }
.collab-slide-konva__savetag.saving { border-color: rgba(24,90,189,0.32); color: #185abd; background: rgba(24,90,189,0.08); }
.collab-slide-konva__title-btn { display: inline-flex; align-items: center; gap: 6px; min-height: 28px; padding: 0 11px; border: 1px solid #185abd; border-radius: 5px; background: #185abd; color: white; cursor: pointer; font-size: 11px; font-weight: 650; }
.collab-slide-konva__title-btn:hover { background: #124a9e; }
.collab-slide-konva__title-btn:disabled { opacity: .45; cursor: not-allowed; }

/* v0.7.135 — Group separators: dark-theme aware.
 * Original `#d3d7df` is light-theme only and invisible on the dark panel.
 * GenOffice uses `rgba(255,255,255,0.08)` for dark separator lines.
 * The light-theme variant is moved to a [data-rb-theme="light"] scope below. */
.collab-slide-konva[data-rb-theme="dark"] .collab-slide-konva__ribbon-groups > .collab-slide-konva__tool-group ~ .collab-slide-konva__tool-group { 
  border-left: 1px solid rgba(255, 255, 255, 0.08); 
}
.collab-slide-konva[data-rb-theme="light"] .collab-slide-konva__ribbon-groups > .collab-slide-konva__tool-group ~ .collab-slide-konva__tool-group { 
  border-left: 1px solid #d3d7df; 
}
/* v0.7.144 — GenOffice-spec overrides (apps/slides/src/renderer/styles.css).
   Align the slide-editor ribbon with the reference: standard 28×30 icons,
   12px fonts, 4px gap, 2px 4px group padding. The horizontal scroll on the
   panel absorbs the wider total; the canvas is unaffected. */
.collab-slide-konva__tool-group.ribbon-group { padding: 2px 4px !important; }
.collab-slide-konva__tool-group.ribbon-group > .ribbon-group-items { gap: 2px !important; }
.collab-slide-konva__tool-group.ribbon-group .rb-big { padding: 4px 7px 6px !important; font-size: 12px !important; gap: 4px !important; }
.collab-slide-konva__tool-group.ribbon-group .rb-big-icon { min-height: 28px !important; padding: 3px 4px !important; }
.collab-slide-konva__tool-group.ribbon-group .rb-small { padding: 3px 8px !important; font-size: 12px !important; }
.collab-slide-konva__tool-group.ribbon-group .rb-icon { width: 32px !important; height: 30px !important; min-width: 32px !important; padding: 0 4px !important; font-size: 15px !important; box-sizing: border-box !important; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__tool-group { position: relative; display: flex; flex-direction: column; justify-content: flex-start; gap: 0; padding: 1px 6px 0; border-right: 0; flex: none; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__tool-group:first-child { padding-left: 1px; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__group-label { position: static; display: block; order: 2; height: 13px; margin: 1px 3px 0; color: #7a8492; font-size: 9px; line-height: 13px; letter-spacing: .02em; text-align: center; }
/* v0.7.144 — slide-konva-scoped override: max-width content (GenOffice pattern). */
.collab-slide-konva__ribbon-groups .ribbon-group-items {
  display: flex;
  align-items: center;
  gap: 2px;
  flex: 1 1 auto;
  min-height: 0;
  max-width: max-content;
  align-self: flex-start;
}
.collab-slide-konva__ribbon-groups .collab-slide-konva__tool-btn { min-width: 46px; min-height: 48px; padding: 3px 6px 2px; border: 1px solid transparent; border-radius: 4px; background: transparent; color: #303844; cursor: pointer; flex-direction: column; font: 11px/1.15 inherit; white-space: nowrap; flex-shrink: 0; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__tool-btn svg { width: 19px; height: 19px; color: #344054; flex: none; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__tool-btn:hover:not(:disabled) { border-color: #cbdaf1; background: #f1f6fe; color: #185abd; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__tool-btn:hover:not(:disabled) svg { color: #185abd; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__tool-btn:disabled { opacity: .38; cursor: not-allowed; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__tool-btn--big { min-width: 66px; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__tool-btn--big svg { width: 22px; height: 22px; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__shape-glyph { color: #344054; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__ribbon-hint { color: #687385; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__layout-select { color: #5c6675; }
.collab-slide-konva__ribbon-groups .collab-slide-konva__layout-dropdown { border-color: #c9d0da; background: #fff; color: #303844; }
.collab-slide-konva__body { position: relative; flex: 1 1 0; min-height: 360px; display: flex; background: #eef1f5; overflow: hidden; }
.collab-slide-konva__recovery { position: absolute; z-index: 3; top: 0; right: 0; left: 0; padding: 8px 14px; margin: 0; border-bottom: 1px solid rgba(244,200,121,.18); background: rgba(92,69,26,.86); color: #f5d38e; font-size: 11px; }
.collab-slide-konva__thumbs { width: 156px; flex: 0 0 156px; min-width: 0; padding: 14px 10px 26px; border-right: 1px solid var(--app-border); background: var(--slide-pane-bg, var(--app-surface-bg, #fff)); color: var(--app-text, #1f232b); gap: 11px; overflow-y: auto; overflow-x: hidden; }
.collab-slide-konva__thumb { width: 100%; min-height: 94px; position: relative; display: grid; grid-template-columns: 20px minmax(0, 1fr); grid-template-rows: auto auto auto; align-items: center; gap: 4px; padding: 6px; border: 1px solid rgba(255,255,255,.1); border-radius: 6px; outline: none; background: #2c313b; color: #bec7d2; cursor: pointer; text-align: left; }
.collab-slide-konva__thumb-canvas { grid-column: 2; grid-row: 1 / span 2; width: 100%; height: 78px; display: flex; align-items: center; justify-content: center; border: 1px solid rgba(0,0,0,.35); border-radius: 2px; background: #f9fbfd; box-shadow: 0 2px 5px rgba(0,0,0,.24); overflow: hidden; }
.collab-slide-konva__thumb-svg { width: 100%; height: 100%; display: block; }
.collab-slide-konva__thumb.active { border-color: var(--slide-accent); background: rgba(90,168,255,.15); box-shadow: 0 0 0 1px rgba(90,168,255,.2); }
.collab-slide-konva__thumb:hover { border-color: rgba(255,255,255,.28); }
.collab-slide-konva__thumb-num { align-self: start; padding-top: 4px; color: #8994a3; font-size: 10px; text-align: center; }
.collab-slide-konva__thumb-title { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #cdd5df; font-size: 10px; }
.collab-slide-konva__iconbtn { position: relative; z-index: 1; grid-row: 2; border: 0; background: transparent; color: #9da8b6; cursor: pointer; font-size: 11px; padding: 2px; }
.collab-slide-konva__iconbtn.danger { color: #ed8990; }
.collab-slide-konva__stage-wrap { flex: 1; min-width: 0; min-height: 0; padding: 18px; background: var(--slide-stage-bg, var(--app-surface-raised, #eef1f5)); gap: 10px; overflow: auto; display: flex; flex-direction: column; align-items: center; justify-content: center; }
.collab-slide-konva__zoom-info { flex: 0 0 auto; align-self: flex-end; margin: 0 0 8px; color: #8d98a7; font-size: 10px; }
.collab-slide-konva__stage { max-width: 100%; max-height: 100%; border: 1px solid rgba(0,0,0,.42); background: white; box-shadow: 0 18px 40px rgba(0,0,0,.35), 0 2px 5px rgba(0,0,0,.24); position: relative; overflow: hidden; flex: 0 0 auto; }
.collab-slide-konva__inspector { width: 258px; flex: 0 0 258px; min-width: 0; padding: 16px 14px; border-left: 1px solid var(--app-border); background: var(--slide-pane-bg, var(--app-surface-bg, #fff)); color: var(--app-text, #1f232b); overflow-y: auto; }
.collab-slide-konva__inspector h3 { padding-bottom: 11px; margin: 0 0 13px; border-bottom: 1px solid var(--app-border); color: var(--app-text, #1f232b); font-size: 12px; }
.collab-slide-konva__inspector label { color: var(--app-text-muted, #6b7280); }
.collab-slide-konva__inspector input, .collab-slide-konva__inspector textarea { border-color: var(--app-border); outline: none; background: var(--app-surface-raised, #fff); color: var(--app-text, #1f232b); }
.collab-slide-konva__inspector input:focus, .collab-slide-konva__inspector textarea:focus { border-color: rgba(90,168,255,.7); box-shadow: 0 0 0 2px rgba(90,168,255,.12); }
/* Bottom panels (notes / animations / comments): light surface. The
# collapsed state (data-collapsed="true") hides the body so canvas reclaims
# the space; clicking the header bar toggles it. The default state is
# collapsed so a fresh page lands with maximum canvas area. */
.collab-slide-konva__notes, .collab-slide-konva__animations, .collab-comments { border-top: 1px solid var(--app-border, #e1e4e8); background: var(--app-surface-raised, #fff); color: var(--app-text, #1f232b); }
.collab-slide-konva__notes-header, .collab-slide-konva__animations-header, .collab-comments__header { display: flex; align-items: center; gap: 8px; padding: 8px 14px; cursor: pointer; user-select: none; }
.collab-slide-konva__notes-header:hover, .collab-slide-konva__animations-header:hover, .collab-comments__header:hover { background: var(--app-surface, #f6f7f9); }
.collab-slide-konva__notes-header > span:first-child, .collab-slide-konva__animations-header > span:first-child { font-size: 12px; font-weight: 600; }
.collab-slide-konva__notes[data-collapsed="true"] > :not(.collab-slide-konva__notes-header),
.collab-slide-konva__animations[data-collapsed="true"] > :not(.collab-slide-konva__animations-header),
.collab-comments[data-collapsed="true"] > :not(.collab-comments__header) { display: none; }
/* v0.7.198 — collapsed state: shrink the header bar to a thin 22px pill so
   the canvas reclaims most of the viewport. The 3 collapsed headers used to
   consume ~143px (notes 67 + animations 37 + comments 39); now they take 66px
   combined, freeing ~77px of stage area. Hover/click reopens the panel. */
.collab-slide-konva__notes[data-collapsed="true"] > .collab-slide-konva__notes-header,
.collab-slide-konva__animations[data-collapsed="true"] > .collab-slide-konva__animations-header {
  padding: 2px 12px;
  font-size: 11px;
  min-height: 22px;
  border-top: 1px solid var(--app-border, #e5e7eb);
  background: transparent;
  color: var(--app-text-muted, #6b7280);
}
.collab-slide-konva__notes[data-collapsed="true"] > .collab-slide-konva__notes-header:hover,
.collab-slide-konva__animations[data-collapsed="true"] > .collab-slide-konva__animations-header:hover {
  background: var(--app-surface-hover, #f3f4f6);
  color: var(--app-text, #1f232b);
}
.collab-slide-konva__notes-textarea, .collab-slide-konva__animations-select, .collab-slide-konva__animations-input { border: 1px solid var(--app-border, #e1e4e8); background: var(--app-surface, #f6f7f9); color: var(--app-text, #1f232b); border-radius: 4px; }
.collab-slide-konva__notes-textarea:focus, .collab-slide-konva__animations-select:focus, .collab-slide-konva__animations-input:focus { border-color: var(--app-accent, #185abd); box-shadow: 0 0 0 2px rgba(24,90,189,0.16); outline: none; }
.collab-slide-konva__notes-textarea { padding: 6px 8px; }
.collab-slide-konva__error { padding: 9px 14px; margin: 0; background: #3b2024; color: #ffadb4; font-size: 11px; }
.collab-slide-konva__loading { min-height: 220px; display: grid; place-items: center; background: #eef1f5; color: #98a4b3; font-size: 12px; }
@media (max-width: 1120px) {
  .collab-slide-konva__thumbs { width: 160px; flex-basis: 160px; }
  .collab-slide-konva__inspector { width: 220px; flex-basis: 220px; }
  .collab-slide-konva__tool-group--arrange { display: none; }
  .collab-slide-konva__context-actions--selection { display: none; }
}
@media (max-width: 820px) {
  .collab-slide-konva__thumbs { width: 124px; flex-basis: 124px; }
  .collab-slide-konva__inspector { display: none; }
  .collab-slide-konva__title-actions .collab-slide-konva__connection, .collab-slide-konva__title-actions .collab-slide-konva__savetag { display: none; }
  .collab-slide-konva__tabs { padding: 0 8px; }
  .collab-slide-konva__tab-btn { padding: 0 8px 9px; font-size: 10px; }
}
/* v0.7.119 — dark chrome when slide is dark. Reuses the dark palette
   the thumb sidebar / stage-wrap / inspector already use. */
.collab-slide-konva[data-rb-theme='dark'] {
  background: #1E1E1E;
  color: #d9e1eb;
}
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__titlebar {
  background: #1E1E1E;
  border-bottom: 1px solid #3a3a3a;
  color: #d9e1eb;
}
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__title-actions,
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__file-meta,
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__title { color: #f0f4f8; }
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__file-subtitle { color: #8d98a7; }
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__connection,
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__savetag {
  border-color: rgba(255, 255, 255, 0.10);
  background: #252525;
  color: #b6c0cd;
}
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__title-btn {
  background: #2a2a2a;
  color: #e6eaf2;
  border-color: rgba(255, 255, 255, 0.08);
}
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__title-btn:hover { background: #353b46; }
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__statusbar {
  background: #1E1E1E;
  color: #b6c0cd;
  border-top: 1px solid #3a3a3a;
}
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__statusbar-item,
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__statusbar-btn { color: #b6c0cd; }
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__body { background: #1e1e1e; }
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__notes,
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__animations,
.collab-slide-konva[data-rb-theme='dark'] .collab-comments {
  background: #1E1E1E;
  color: #d9e1eb;
  border-top-color: #3a3a3a;
}


/* v0.7.131 — Dark-mode group separator (GenOffice style: thin 1px line spanning
   the panel height). The explicit <div class="ribbon-sep"> siblings sit in
   .collab-slide-konva__ribbon-groups and collapse to 0 width under flex stretch
   — use the border-left on the next tool-group as the authoritative separator. */
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__ribbon-groups > .collab-slide-konva__tool-group ~ .collab-slide-konva__tool-group { border-left-color: rgba(255, 255, 255, 0.08); }
/* v0.7.144 — allow explicit .ribbon-sep divs to be visible (GenOffice spec).
   The border-left above is the fall-back; explicit seps sit alongside it. */
.collab-slide-konva[data-rb-theme='dark'] .collab-slide-konva__ribbon-groups .ribbon-sep { display: block; background: var(--rb-border, #3a3a3a); width: 1px; align-self: stretch; margin: 2px 6px; flex-shrink: 0; }

/* v0.7.139 — AI big button (GenOffice pattern: 紫色 glyph on transparent plate).
   The 'rb-big-ai' modifier keeps the icon row clear while hovering. */
.rb-big.rb-big-ai { color: var(--rb-text); }
.rb-big.rb-big-ai .rb-big-icon { color: var(--color-brand-secondary, #6ba1ff); }
.rb-big.rb-big-ai:hover:not(:disabled) .rb-big-icon { background: var(--rb-hover); }

/* v0.7.141 — Font group: outer column stack hosts 2 horizontal rows
   (GenOffice .rb-col + .rb-row pattern). Bug in v0.7.139: .rb-font-row
   was column flex, so row 2's 3 buttons (size + grow + shrink) stacked
   vertically inside an outer row flex. The fix wraps them in a column
   stack with two row children. */
.rb-font-stack { display: flex; flex-direction: column; gap: 2px; flex: 1; min-width: 0; align-self: stretch; }
.rb-font-row { display: flex; align-items: center; gap: 2px; min-width: 0; }
.rb-font-row--name { justify-content: stretch; }
.rb-font-row--name .rb-drop-wrap { flex: 1; min-width: 0; }
.rb-font-row--ctrl { justify-content: flex-start; }
.rb-font-btn {
  min-width: 0;
  justify-content: space-between;
  padding: 2px 7px;
  border: 1px solid var(--rb-border);
  background: var(--rb-chrome-bg-deep, #1e1e1e);
  font-size: 12px;
  flex: 1;
}
.rb-font-btn--size { flex: 0 0 50px; min-width: 0; }
.rb-font-btn:hover:not(:disabled) { border-color: var(--rb-accent); background: var(--rb-hover); }
.rb-font-btn .rb-font-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; flex: 1; }

/* v0.7.144 — Paragraph group: 2-row layout (GenOffice .rb-col + .rb-row).
   Each row holds 4 icons with proper GenOffice gap. */
.rb-arrange-row {
  display: flex;
  align-items: center;
  gap: 2px;
  flex: 1;
  justify-content: center;
}

/* v0.7.119 — lock label font-size in scoped context (more specific selectors
   in this file otherwise inherit 14px). */
/* v0.7.134 — Hide group labels to match GenOffice's compact icon-only panel.
 * Group identity remains via data-tip tooltips + visual grouping + sep dividers.
 * Re-enable by removing this rule (or commenting it out). */
.collab-slide-konva__ribbon-groups .ribbon-group-label--visible {
  display: none !important;
}

</style>
    return { index: Number(obj.index ?? 0), width, height, background, shapes }
