<!--
  CollabDocProEditor — v0.7.26 DOC-kind collaborative editor with real
  .docx byte round-trip.

  Architecture:
   1. Open: fetch the latest .docx bytes via REST
            → docxAdapter.openDocx(bytes) returns ParsedDocFull
            → render each block as a TipTap paragraph with
              data-docx-index so we can re-locate it on save.
   2. Edit: TipTap StarterKit + CollaborationCursor over Yjs.
            On update, debounced 1.5s, walk TipTap paragraphs and
            patchParagraphText() the matching docx-engine block.
   3. Save: docxAdapter.saveDocxBytes(parsed) → REST upload.
   4. Realtime: per-paragraph Y.Text binding via TipTap's `field` config
      so two clients editing different paragraphs converge cleanly.

  Format coverage today: paragraph/heading/listItem blocks round-trip
  faithfully. Tables, images, and shapes stay read-only on save (their
  byte spans are preserved verbatim because we only mutate runs inside
  those blocks — the patch path leaves anchor XML untouched).
-->
<template>
  <div class="collab-doc-pro">
    <CollabEditorRibbon
      v-model="activeTab"
      :tabs="docRibbonTabs"
      aria-label="文档工具栏"
      test-id-prefix="doc"
      :collapsible="true"
    >
      <!-- ===== 文件 tab ===== -->
      <template #file>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">文件</span>
          <button data-testid="doc-download-btn" class="collab-doc-pro__btn" type="button" :disabled="downloading" @click="onDownload">
            {{ downloading ? '下载中...' : '下载 .docx' }}
          </button>
          <button data-testid="doc-upload-btn" class="collab-doc-pro__btn" type="button" :disabled="uploading" @click="triggerDocUpload">
            {{ uploading ? '上传中...' : '上传 .docx / .txt / .md' }}
          </button>
          <button data-testid="doc-save-btn" class="collab-doc-pro__btn" type="button" :disabled="uploading" @click="onForceSave">
            {{ uploading ? '保存中...' : '立即保存' }}
          </button>
          <button data-testid="doc-history-btn" class="collab-doc-pro__btn" type="button" @click="onToggleHistory">版本历史</button>
        </div>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">页面</span>
          <button data-testid="doc-page-setup-btn" class="collab-doc-pro__btn" type="button" @click="openSectionsModal">页面设置</button>
          <button data-testid="doc-hf-btn-ribbon" class="collab-doc-pro__btn" type="button" @click="openHfModal">页眉页脚</button>
        </div>
      </template>

      <!-- ===== 开始 tab (字体/段落/样式/撤销重做) ===== -->
      <template #home>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">撤销</span>
          <button data-testid="doc-undo-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="runNode('undo')" title="撤销 (Ctrl+Z)">
            <CollabIcon name="IconUndo" :size="28" /><span>撤销</span>
          </button>
          <button data-testid="doc-redo-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="runNode('redo')" title="重做 (Ctrl+Y)">
            <CollabIcon name="IconRedo" :size="28" /><span>重做</span>
          </button>
        </div>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">段落</span>
          <button data-testid="doc-align-left-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onAlign('left')" title="左对齐">
            <CollabIcon name="IconAlignLeft" :size="28" /><span>左对齐</span>
          </button>
          <button data-testid="doc-align-center-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onAlign('center')" title="居中">
            <CollabIcon name="IconAlignCenter" :size="28" /><span>居中</span>
          </button>
          <button data-testid="doc-align-right-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onAlign('right')" title="右对齐">
            <CollabIcon name="IconAlignRight" :size="28" /><span>右对齐</span>
          </button>
        </div>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">格式</span>
          <button data-testid="doc-case-ribbon-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" data-testid-case="case" :disabled="!editor" title="大小写切换 (Shift+F3)" @click="onCycleCase">
            <CollabIcon name="IconChangeCase" :size="28" /><span>大小写</span>
          </button>
          <button data-testid="doc-clear-fmt-ribbon-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" :disabled="!editor || !canClearFormat" title="清除格式 (Ctrl+Space)" @click="onClearFormat">
            <CollabIcon name="IconClearFormat" :size="28" /><span>清除</span>
          </button>
        </div>
      </template>

      <!-- ===== 插入 tab (表格/图片/链接/页码/页眉页脚/分页符/数学公式) ===== -->
      <template #insert>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">插入</span>
          <button data-testid="doc-insert-table-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onInsertTable" title="插入表格">
            <CollabIcon name="IconTable" :size="28" /><span>表格</span>
          </button>
          <button data-testid="doc-insert-image-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onInsertImageUrl" title="插入图片">
            <CollabIcon name="IconPicture" :size="28" /><span>图片</span>
          </button>
          <button data-testid="doc-insert-image-file-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onInsertImageFile" title="插入本地图片">
            <CollabIcon name="IconPaperclip" :size="28" /><span>本地</span>
          </button>
          <button data-testid="doc-insert-link-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" :disabled="!editor" @click="onSetLink" title="插入链接">
            <CollabIcon name="IconLink" :size="28" /><span>链接</span>
          </button>
        </div>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">布局</span>
          <button data-testid="doc-page-break-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onInsertPageBreak">
            <CollabIcon name="IconPageBreak" :size="28" /><span>分页符</span>
          </button>
          <button data-testid="doc-math-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onOpenMath">
            <CollabIcon name="IconCaret" :size="28" /><span>公式</span>
          </button>
        </div>
      </template>

      <!-- ===== 绘图 tab (形状/笔迹) ===== -->
      <template #draw>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">形状</span>
          <button data-testid="doc-shape-rect-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" :disabled="!editor" @click="onInsertShape('rect')" title="插入矩形">
            <CollabIcon name="IconShapes" :size="28" /><span>矩形</span>
          </button>
          <button data-testid="doc-shape-ellipse-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" :disabled="!editor" @click="onInsertShape('ellipse')" title="插入椭圆">
            <CollabIcon name="IconShapes" :size="28" /><span>椭圆</span>
          </button>
        </div>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">笔迹</span>
          <button data-testid="doc-pen-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" disabled title="手写笔（即将推出）">
            <CollabIcon name="IconPen" :size="28" /><span>笔</span>
          </button>
          <button data-testid="doc-highlighter-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" disabled title="荧光笔（即将推出）">
            <CollabIcon name="IconHighlighterPen" :size="28" /><span>荧光</span>
          </button>
          <button data-testid="doc-eraser-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" disabled title="橡皮（即将推出）">
            <CollabIcon name="IconEraser" :size="28" /><span>橡皮</span>
          </button>
        </div>
      </template>

      <!-- ===== 设计 tab (主题) ===== -->
      <template #design>
        <CollabDocThemePanel
          :active-theme="activeDocTheme"
          @apply="onApplyDocTheme"
        />
      </template>

      <!-- ===== 审阅 tab (修订/记录修订/对比/保护) ===== -->
      <template #review>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">修订</span>
          <button data-testid="doc-track-changes-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onToggleTrackChanges">
            <CollabIcon name="IconTrackChanges" :size="28" /><span>{{ trackChangesOn ? '记录中' : '记录修订' }}</span>
          </button>
          <button data-testid="doc-revisions-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="openRevisionsPanel">
            <CollabIcon name="IconComment" :size="28" /><span>修订 ({{ revisionCount }})</span>
          </button>
        </div>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">比较与保护</span>
          <button data-testid="doc-compare-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="openCompareModal">
            <CollabIcon name="IconCompare" :size="28" /><span>对比</span>
          </button>
          <button data-testid="doc-protect-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="openProtectModal">
            <CollabIcon name="IconLock" :size="28" /><span>{{ protectionEnabled ? '已保护' : '保护' }}</span>
          </button>
        </div>
      </template>

      <!-- ===== 视图 tab (大纲/查找/标尺/网格线/缩放) ===== -->
      <template #view>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">导航</span>
          <button data-testid="doc-outline-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onToggleOutline">
            <CollabIcon name="IconNavPane" :size="28" /><span>大纲</span>
          </button>
          <button data-testid="doc-find-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="openFindPanel">
            <CollabIcon name="IconReplace" :size="28" /><span>查找</span>
          </button>
        </div>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">视图</span>
          <button data-testid="doc-ruler-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="rulerVisible = !rulerVisible">
            <CollabIcon name="IconRuler" :size="28" /><span>标尺</span>
          </button>
          <button data-testid="doc-gridlines-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="gridlinesVisible = !gridlinesVisible">
            <CollabIcon name="IconGridlines" :size="28" /><span>网格</span>
          </button>
        </div>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">缩放</span>
          <button data-testid="doc-zoom-out-ribbon-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onZoomOut" title="缩小">
            <CollabIcon name="IconZoomOut" :size="28" /><span>缩小</span>
          </button>
          <button data-testid="doc-zoom-in-ribbon-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onZoomIn" title="放大">
            <CollabIcon name="IconZoomIn" :size="28" /><span>放大</span>
          </button>
          <button data-testid="doc-zoom-100-btn" class="collab-doc-pro__btn collab-doc-pro__btn--icon" type="button" @click="onZoom100" title="100%">
            <CollabIcon name="IconZoom100" :size="28" /><span>100%</span>
          </button>
        </div>
      </template>

      <!-- ===== AI tab ===== -->
      <template #ai>
        <div class="collab-doc-pro__ribbon-group">
          <span class="collab-doc-pro__ribbon-group-label">AI 助手</span>
          <button data-testid="doc-ai-btn" class="collab-doc-pro__btn collab-doc-pro__btn--ai collab-doc-pro__btn--icon" type="button" :disabled="!aiOriginal" @click="onOpenAi">
            <CollabIcon name="IconAiPanel" :size="28" /><span>问 AI</span>
          </button>
        </div>
      </template>
    </CollabEditorRibbon>

    <div class="collab-doc-pro__toolbar">
      <span class="collab-doc-pro__title">{{ title }}</span>
      <span class="collab-doc-pro__kind">{{ kindLabel }}</span>
      <span class="collab-doc-pro__connection" :class="{ connected: connected && !saveError }">
        {{ connectionLabel }}
      </span>
      <span class="collab-doc-pro__savetag" :class="savetagClass">
        {{ saveLabel }}
      </span>
      <button class="collab-doc-pro__btn" :disabled="downloading" @click="onDownload">
        {{ downloading ? '下载中...' : '下载 .docx' }}
      </button>
      <button class="collab-doc-pro__btn" :disabled="uploading" @click="triggerDocUpload" type="button">
        {{ uploading ? '上传中...' : '上传 .docx / .txt / .md' }}
      </button>
      <input
        ref="docFileInput"
        type="file"
        accept=".docx,.txt,.md"
        style="display:none"
        @change="onUploadDocFile"
      />
      <button class="collab-doc-pro__btn" :disabled="uploading" @click="onForceSave">
        {{ uploading ? '保存中...' : '立即保存' }}
      </button>
      <button class="collab-doc-pro__btn collab-doc-pro__btn--ai" :disabled="!aiOriginal" @click="onOpenAi" type="button">
        问 AI
      </button>
      <!-- v0.7.42 — Math formula (LaTeX → MathML) via docxAdapter.latexToDocxMath -->
      <button class="collab-doc-pro__btn" type="button" data-testid="doc-math-btn" @click="onOpenMath">
        公式
      </button>
      <button class="collab-doc-pro__btn" type="button" data-testid="doc-page-break-btn" @click="onInsertPageBreak">
        分页符
      </button>
      <button class="collab-doc-pro__btn" type="button" data-testid="doc-hf-btn" @click="openHfModal">
        页眉页脚
      </button>
      <!-- v0.7.67 — DOC document protection (Word "Review > Protect Document") -->
      <button class="collab-doc-pro__btn" type="button" data-testid="doc-protect-btn" @click="openProtectModal">
        {{ protectionEnabled ? '已保护' : '保护文档' }}
      </button>
      <!-- v0.7.106 — DOC track-changes recording toggle (Word "Review > Track Changes"). -->
      <button
        class="collab-doc-pro__btn"
        :class="{ 'collab-doc-pro__btn--active': trackChangesOn }"
        type="button"
        data-testid="doc-track-changes-btn"
        :title="trackChangesOn ? '点击停止记录修订' : '点击开始记录修订（输入会生成 w:ins，删除生成 w:del）'"
        @click="onToggleTrackChanges"
      >
        {{ trackChangesOn ? '● 记录中' : '记录修订' }}
      </button>
      <!-- v0.7.68 — DOC track changes / accept-all / reject-all (Word "Review" ribbon) -->
      <button class="collab-doc-pro__btn" type="button" data-testid="doc-revisions-btn" @click="openRevisionsPanel">
        修订 ({{ revisionCount }})
      </button>
      <!-- v0.7.69 — DOC compare (Word "Review > Compare") -->
      <button class="collab-doc-pro__btn" type="button" data-testid="doc-compare-btn" @click="openCompareModal">
        对比文档
      </button>
      <!-- v0.7.71 — DOC heading outline (Word "View > Navigation Pane") -->
      <button
        class="collab-doc-pro__btn"
        type="button"
        data-testid="doc-outline-btn"
        :disabled="!editor"
        @click="onToggleOutline"
      >
        {{ outlineOpen ? '关闭大纲' : '大纲' }}
      </button>
      <!-- v0.7.72 — DOC page setup / multi-section preview -->
      <button
        class="collab-doc-pro__btn"
        type="button"
        data-testid="doc-sections-btn"
        :disabled="!doc"
        @click="openSectionsModal"
      >
        页面设置
      </button>
      <!-- v0.7.73 — DOC find / replace (Word "Home > Find" / Ctrl+F) -->
      <button
        class="collab-doc-pro__btn"
        type="button"
        data-testid="doc-find-btn"
        :disabled="!editor"
        @click="openFindPanel"
      >
        {{ findOpen ? '关闭查找' : '查找' }}
      </button>
      <button class="collab-doc-pro__btn" :disabled="!doc" @click="onToggleHistory" type="button">
        {{ historyOpen ? '关闭历史' : '版本历史' }}
      </button>
      <span class="collab-doc-pro__peers">
        <span
          v-for="p in peers"
          :key="p.clientId"
          class="collab-doc-pro__peer"
          :class="{ 'collab-doc-pro__peer--selecting': remoteSelectionFor(p.clientId) }"
          :style="{ backgroundColor: p.color }"
          :title="peerTitle(p)"
        >{{ initialOf(p.displayName) }}</span>
      </span>
    </div>
    <div v-if="loading" class="collab-doc-pro__loading">加载文档中…</div>
    <div v-if="!loading && loadError" class="collab-doc-pro__error">加载失败：{{ loadError }}</div>
    <div v-if="!loading && !loadError && loadRecovery" class="collab-doc-pro__recovery">{{ loadRecovery }}</div>
    <div v-if="!loading && !loadError" class="collab-doc-pro__main">
      <div class="collab-doc-pro__formatbar" v-if="editor">
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('bold') }" @click="runMark('bold')" type="button" title="粗体 (Ctrl+B)"><b>B</b></button>
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('italic') }" @click="runMark('italic')" type="button" title="斜体 (Ctrl+I)"><i>I</i></button>
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('strike') }" @click="runMark('strike')" type="button" title="删除线"><s>S</s></button>
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('code') }" @click="runMark('code')" type="button" title="行内代码"><code>&lt;/&gt;</code></button>
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('underline') }" @click="runMark('underline')" type="button" title="下划线"><u>U</u></button>
        <span class="collab-doc-pro__sep"></span>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('heading', { level: 1 }) }" @click="runHeading(1)" type="button" title="一级标题">H1</button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('heading', { level: 2 }) }" @click="runHeading(2)" type="button" title="二级标题">H2</button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('heading', { level: 3 }) }" @click="runHeading(3)" type="button" title="三级标题">H3</button>
        <span class="collab-doc-pro__sep"></span>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('bulletList') }" @click="runNode('toggleBulletList')" type="button" title="无序列表">• 列表</button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('orderedList') }" @click="runNode('toggleOrderedList')" type="button" title="有序列表">1. 列表</button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('taskList') }" @click="runNode('toggleTaskList')" type="button" title="任务列表"><CollabIcon name="IconCheckbox" :size="14" /></button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('blockquote') }" @click="runNode('toggleBlockquote')" type="button" title="引用"><CollabIcon name="IconQuote" :size="14" /></button>
        <button class="collab-doc-pro__fmt" :class="{ active: isNodeActive('codeBlock') }" @click="runNode('toggleCodeBlock')" type="button" title="代码块"><CollabIcon name="IconCode" :size="14" /></button>
        <span class="collab-doc-pro__sep"></span>
        <button class="collab-doc-pro__fmt" :class="{ active: isMarkActive('link') }" @click="onSetLink" type="button" title="链接"><CollabIcon name="IconLink" :size="14" /></button>
        <button class="collab-doc-pro__fmt" @click="onInsertTable" type="button" title="插入表格"><CollabIcon name="IconTable" :size="14" /></button>
        <button class="collab-doc-pro__fmt" @click="onSetColumnWidth" type="button" title="调整列宽 (px)"><CollabIcon name="IconColumnWidth" :size="14" /> 列宽</button>
        <button class="collab-doc-pro__fmt" @click="onApplyTablePreset" type="button" title="表格样式 (蓝/灰/无)"><CollabIcon name="IconPalette" :size="14" /> 样式</button>
        <button class="collab-doc-pro__fmt" @click="onToggleRepeatHeader" type="button" title="跨页重复表头"><CollabIcon name="IconHeaderRow" :size="14" /> 表头</button>
        <button class="collab-doc-pro__fmt" @click="onInsertImageUrl" type="button" title="插入图片"><CollabIcon name="IconPicture" :size="14" /></button>
      <button class="collab-doc-pro__btn" @click="onInsertImageFile" type="button" title="插入本地图片"><CollabIcon name="IconPicture" :size="14" /> 本地</button>
      <input ref="fileImageInput" type="file" accept="image/*" style="display:none" @change="onImageFileChosen" />
        <button class="collab-doc-pro__fmt" @click="onAlign('left')" type="button" title="左对齐"><CollabIcon name="IconAlignLeft" :size="14" /></button>
        <button class="collab-doc-pro__fmt" @click="onAlign('center')" type="button" title="居中"><CollabIcon name="IconAlignCenter" :size="14" /></button>
        <button class="collab-doc-pro__fmt" @click="onAlign('right')" type="button" title="右对齐"><CollabIcon name="IconAlignRight" :size="14" /></button>
        <span class="collab-doc-pro__sep"></span>
        <button class="collab-doc-pro__fmt" @click="runNode('undo')" type="button" title="撤销 (Ctrl+Z)"><CollabIcon name="IconUndo" :size="14" /></button>
        <button class="collab-doc-pro__fmt" @click="runNode('redo')" type="button" title="重做 (Ctrl+Y)"><CollabIcon name="IconRedo" :size="14" /></button>
        <button class="collab-doc-pro__fmt" type="button" data-testid="doc-move-up" :disabled="!editor" title="上移段落 (Alt+Shift+↑)" @click="onMoveBlock(-1)">▲</button>
        <button class="collab-doc-pro__fmt" type="button" data-testid="doc-move-down" :disabled="!editor" title="下移段落 (Alt+Shift+↓)" @click="onMoveBlock(1)">▼</button>
        <button class="collab-doc-pro__fmt" type="button" data-testid="doc-case" :disabled="!editor" title="大小写切换 (Shift+F3)" @click="onCycleCase"><CollabIcon name="IconChangeCase" :size="14" /></button>
        <button class="collab-doc-pro__fmt" type="button" data-testid="doc-clear-fmt" :disabled="!editor || !canClearFormat" title="清除格式 (Ctrl+Space)" @click="onClearFormat"><CollabIcon name="IconClearFormat" :size="14" /></button>
      </div>
      <div class="collab-doc-pro__surface-wrap">
      <CollabDocRuler
        v-if="rulerVisible"
        :visible="rulerVisible"
        :ruler-width="820"
        :left-margin="rulerLeftMargin"
        :right-margin="rulerRightMargin"
        @update:left-margin="rulerLeftMargin = $event"
        @update:right-margin="rulerRightMargin = $event"
      />
      <EditorContent :editor="editor" class="collab-doc-pro__surface" />
      <CollabAiPolishDialog
        v-if="aiOpen"
        :open="aiOpen"
        :anchor="aiAnchor"
        :original="aiOriginal"
        @close="aiOpen = false"
        @accept="onAcceptAi"
      />
      </div>
      <!-- v0.7.42 — Math formula dialog (LaTeX in, MathML out) -->
      <div v-if="mathOpen" class="collab-doc-pro__math-bg" @click="mathOpen = false">
        <div class="collab-doc-pro__math" @click.stop>
          <h3>插入公式 (LaTeX)</h3>
          <textarea
            v-model="mathLatex"
            class="collab-doc-pro__math-input"
            placeholder="例: \frac{a}{b} 或 x^2 + y^2 = z^2"
            rows="3"
            data-testid="doc-math-input"
          ></textarea>
          <div class="collab-doc-pro__math-preview" data-testid="doc-math-preview" v-html="mathPreviewHtml"></div>
          <p v-if="mathError" class="collab-doc-pro__math-error" data-testid="doc-math-error">{{ mathError }}</p>
        </div>
      </div>
      <!-- v0.7.66 — DOC header/footer editor -->
      <div v-if="hfOpen" class="collab-doc-pro__math-bg" @click="hfOpen = false">
        <div class="collab-doc-pro__math" @click.stop>
          <h3>页眉页脚</h3>
          <label>页眉文本：
            <input v-model="headerTextInput" class="collab-doc-pro__math-input" placeholder="默认页眉" />
          </label>
          <label>页脚文本：
            <input v-model="footerTextInput" class="collab-doc-pro__math-input" placeholder="默认页脚" />
          </label>
          <label>
            <input type="checkbox" v-model="footerPageNumberInput" />
            页脚自动追加页码 (PAGE 字段)
          </label>
          <div class="collab-doc-pro__math-actions">
            <button type="button" @click="hfOpen = false">取消</button>
            <button type="button" data-testid="doc-hf-clear" @click="onHfClear">清除</button>
            <button type="button" data-testid="doc-hf-save" @click="onHfSave">保存</button>
          </div>
          <div class="collab-doc-pro__math-actions">
            <button type="button" @click="mathOpen = false">取消</button>
            <button type="button" data-testid="doc-math-insert" @click="onInsertMath">插入文档</button>
          </div>
        </div>
      </div>
      <!-- v0.7.67 — DOC document protection dialog (Word-style) -->
      <div v-if="protectOpen" class="collab-doc-pro__math-bg" @click="protectOpen = false">
        <div class="collab-doc-pro__math collab-doc-pro__protect" @click.stop>
          <h3>保护文档</h3>
          <p class="collab-doc-pro__protect-desc">
            设置编辑限制：跟踪修订 / 仅评论 / 只读 / 表单填写。可选密码保护。
          </p>
          <label class="collab-doc-pro__protect-row">
            <input
              type="checkbox"
              data-testid="doc-protect-enabled"
              :checked="protectPatch.enabled"
              @change="onProtectEnabledChange(($event.target as HTMLInputElement).checked)"
            />
            开启编辑限制
          </label>
          <template v-if="protectPatch.enabled">
            <label class="collab-doc-pro__protect-row">
              限制模式：
              <select
                v-model="protectPatch.mode"
                data-testid="doc-protect-mode"
                @change="protectPatch.mode = ($event.target as HTMLSelectElement).value as any"
              >
                <option value="trackedChanges">跟踪修订</option>
                <option value="comments">仅评论</option>
                <option value="readOnly">只读</option>
                <option value="forms">表单填写</option>
              </select>
            </label>
            <label class="collab-doc-pro__protect-row">
              取消限制密码（保留现有密码时留空）：
              <input
                v-model="protectPatch.unlockPassword"
                type="password"
                data-testid="doc-protect-unlock"
                placeholder="输入现有密码"
              />
            </label>
            <label class="collab-doc-pro__protect-row">
              新密码（清除密码请留空，6 位以上）：
              <input
                v-model="protectPatch.password"
                type="password"
                data-testid="doc-protect-pwd"
                placeholder="至少 6 位"
              />
            </label>
            <label class="collab-doc-pro__protect-row">
              确认密码：
              <input
                v-model="protectPatch.passwordConfirm"
                type="password"
                data-testid="doc-protect-pwd2"
                placeholder="再次输入"
              />
            </label>
            <p v-if="protectPatch.error" class="collab-doc-pro__math-error" data-testid="doc-protect-error">
              {{ protectErrorText }}
            </p>
          </template>
          <div class="collab-doc-pro__math-actions">
            <button type="button" @click="protectOpen = false">取消</button>
            <button type="button" data-testid="doc-protect-clear" @click="onProtectClear" :disabled="!protectPatch.enabled">
              清除保护
            </button>
            <button type="button" data-testid="doc-protect-save" @click="onProtectSave" :disabled="protectPatch.busy">
              {{ protectPatch.busy ? '保存中...' : '应用' }}
            </button>
          </div>
        </div>
      </div>
      <!-- v0.7.68 — DOC track changes / revisions panel -->
      <div v-if="revisionsOpen" class="collab-doc-pro__math-bg" @click="revisionsOpen = false">
        <div class="collab-doc-pro__math collab-doc-pro__revisions" @click.stop>
          <h3>修订记录（接受 / 拒绝）</h3>
          <p class="collab-doc-pro__protect-desc">
            Word 风格的修订面板：按作者分组，一键全部接受或拒绝。
          </p>
          <div v-if="revisions.length === 0" class="collab-doc-pro__revisions-empty">
            当前文档无修订记录
          </div>
          <ul v-else class="collab-doc-pro__revisions-list" data-testid="doc-revisions-list">
            <li v-for="(group, idx) in revisionsByAuthor" :key="idx" class="collab-doc-pro__revisions-group">
              <div class="collab-doc-pro__revisions-author">
                <strong>{{ group.author || '匿名' }}</strong>
                <span class="collab-doc-pro__revisions-count">{{ group.count }} 处</span>
              </div>
              <ul class="collab-doc-pro__revisions-items">
                <li v-for="(rev, i) in group.items" :key="i" class="collab-doc-pro__revisions-item">
                  <div class="collab-doc-pro__revisions-item-head">
                    <span class="collab-doc-pro__revisions-kind" :class="'collab-doc-pro__revisions-kind--' + rev.kind">{{ revisionLabel(rev.kind) }}</span>
                    <code class="collab-doc-pro__revisions-snippet">{{ rev.snippet || '（空）' }}</code>
                    <!-- v0.7.106 — per-revision timestamp rendered alongside the snippet. -->
                    <span
                      v-if="rev.date"
                      class="collab-doc-pro__revisions-date"
                      :title="rev.date"
                    >{{ formatRevDate(rev.date) }}</span>
                    <span class="collab-doc-pro__revisions-item-actions">
                      <button type="button" class="collab-doc-pro__revisions-mini" :data-testid="`doc-rev-goto-${idx}-${i}`" @click="onRevisionItemGoto(rev)" title="跳到此处">跳转</button>
                      <button type="button" class="collab-doc-pro__revisions-mini accept" :data-testid="`doc-rev-accept-${idx}-${i}`" @click="onRevisionItemAccept(rev)" title="仅接受此修订">接受</button>
                      <button type="button" class="collab-doc-pro__revisions-mini reject" :data-testid="`doc-rev-reject-${idx}-${i}`" @click="onRevisionItemReject(rev)" title="仅拒绝此修订">拒绝</button>
                    </span>
                  </div>
                </li>
              </ul>
              <div class="collab-doc-pro__math-actions">
                <button type="button" data-testid="doc-revisions-goto" @click="onRevisionGoto(group.items[0])">
                  跳到下一处
                </button>
                <button type="button" data-testid="doc-revisions-accept-author" @click="onAcceptAuthor(group.author)">
                  全部接受
                </button>
                <button type="button" data-testid="doc-revisions-reject-author" @click="onRejectAuthor(group.author)">
                  全部拒绝
                </button>
              </div>
            </li>
          </ul>
          <div class="collab-doc-pro__math-actions">
            <button type="button" data-testid="doc-revisions-accept-all" @click="onAcceptAll" :disabled="revisions.length === 0">
              全部接受
            </button>
            <button type="button" data-testid="doc-revisions-reject-all" @click="onRejectAll" :disabled="revisions.length === 0">
              全部拒绝
            </button>
            <button type="button" @click="revisionsOpen = false">关闭</button>
          </div>
        </div>
      </div>
      <!-- v0.7.69 — DOC compare panel -->
      <div v-if="compareOpen" class="collab-doc-pro__math-bg" @click="compareOpen = false">
        <div class="collab-doc-pro__math collab-doc-pro__compare" @click.stop>
          <h3>对比文档</h3>
          <p class="collab-doc-pro__protect-desc">
            选择另一个 .docx 文件，按段落级别比对两份文档的差异。
          </p>
          <div v-if="!compareOther" class="collab-doc-pro__compare-upload">
            <input
              type="file"
              accept=".docx"
              data-testid="doc-compare-input"
              @change="onCompareFileSelected"
            />
            <p class="collab-doc-pro__protect-desc">（仅解析段落文本，不上传）</p>
          </div>
          <template v-else>
            <div class="collab-doc-pro__compare-meta">
              对比文件：<strong>{{ compareOther.name }}</strong> ·
              <span class="collab-doc-pro__compare-stat added">+{{ compareSummary.added }}</span> ·
              <span class="collab-doc-pro__compare-stat removed">−{{ compareSummary.removed }}</span> ·
              <span class="collab-doc-pro__compare-stat changed">~{{ compareSummary.changed }}</span>
            </div>
            <div v-if="compareSummary.added + compareSummary.removed + compareSummary.changed === 0" class="collab-doc-pro__compare-empty">
              两份文档完全一致
            </div>
            <div v-else class="collab-doc-pro__compare-list" data-testid="doc-compare-list">
              <div v-for="(row, idx) in compareRows" :key="idx" class="collab-doc-pro__compare-row" :class="'collab-doc-pro__compare-row--' + row.kind">
                <span v-if="row.kind === 'same'" class="collab-doc-pro__compare-same">…{{ row.count }} 处未变更…</span>
                <template v-else-if="row.entry">
                  <div v-if="row.entry.kind === 'removed' || row.entry.kind === 'changed'" class="collab-doc-pro__compare-cell collab-doc-pro__compare-cell--left">
                    <span class="collab-doc-pro__compare-label">当前</span>
                    <span>{{ row.entry.left || '（空）' }}</span>
                  </div>
                  <div v-if="row.entry.kind === 'added' || row.entry.kind === 'changed'" class="collab-doc-pro__compare-cell collab-doc-pro__compare-cell--right">
                    <span class="collab-doc-pro__compare-label">对比</span>
                    <span>{{ row.entry.right || '（空）' }}</span>
                  </div>
                </template>
              </div>
            </div>
            <div class="collab-doc-pro__math-actions">
              <button type="button" @click="compareOther = null">重新选择</button>
              <button type="button" @click="compareOpen = false">关闭</button>
            </div>
          </template>
        </div>
      </div>
      <div v-if="historyOpen" class="collab-doc-pro__history">
        <div class="collab-doc-pro__history-head">版本历史</div>
        <div v-if="versions.length === 0" class="collab-doc-pro__history-empty">暂无历史版本</div>
        <div v-for="v in versions" :key="v.version" class="collab-doc-pro__history-row">
          <div class="collab-doc-pro__history-meta">
            <strong>v{{ v.version }}</strong>
            <span>{{ formatBytes(v.size_bytes) }}</span>
            <span>{{ formatTime(v.created_at) }}</span>
          </div>
          <div class="collab-doc-pro__history-actions">
            <button class="collab-doc-pro__btn" @click="onDownloadVersion(v.version)">下载</button>
            <button class="collab-doc-pro__btn primary" @click="onRestoreVersion(v.version)">恢复</button>
          </div>
        </div>
      </div>
      <!-- v0.7.71 — DOC outline panel (heading navigation) -->
      <div v-if="outlineOpen" class="collab-doc-pro__outline" data-testid="doc-outline-panel">
        <div class="collab-doc-pro__outline-head">
          <span>文档大纲</span>
          <span class="collab-doc-pro__outline-count">{{ outlineList.length }} 个标题</span>
        </div>
        <div v-if="outlineList.length === 0" class="collab-doc-pro__outline-empty">文档暂无标题</div>
        <div v-else class="collab-doc-pro__outline-list">
          <button
            v-for="(h, i) in outlineList"
            :key="`${h.pos}-${i}`"
            class="collab-doc-pro__outline-item"
            :class="`collab-doc-pro__outline-item--l${Math.min(h.level, 4)}`"
            :data-tip="h.text"
            :data-testid="`doc-outline-item-${i}`"
            type="button"
            @click="onOutlineJump(h)"
          >{{ h.text }}</button>
        </div>
      </div>
    </div>
    <!-- v0.7.72 — DOC page-setup / multi-section preview modal -->
    <div v-if="sectionsOpen" class="collab-doc-pro__math-bg" @click="closeSectionsModal">
      <div
        class="collab-doc-pro__math collab-doc-pro__sections"
        data-testid="doc-sections-modal"
        @click.stop
      >
        <div class="collab-doc-pro__math-head">页面设置（共 {{ sectionsList.length }} 节）</div>
        <div class="collab-doc-pro__sections-body">
          <div class="collab-doc-pro__sections-list" data-testid="doc-sections-list">
            <button
              v-for="s in sectionsList"
              :key="s.index"
              type="button"
              class="collab-doc-pro__sections-item"
              :class="{ active: sectionsSelected === s.index }"
              :data-testid="`doc-section-item-${s.index}`"
              @click="onSelectSection(s.index)"
            >
              {{ formatSectionSummary(s) }}
            </button>
            <div v-if="sectionsList.length === 0" class="collab-doc-pro__sections-empty">暂无节信息</div>
          </div>
          <div v-if="sectionsList[sectionsSelected]" class="collab-doc-pro__sections-detail" data-testid="doc-sections-detail">
            <div class="collab-doc-pro__sections-row">
              <span class="collab-doc-pro__sections-label">纸张</span>
              <span class="collab-doc-pro__sections-value">{{ paperLabel(sectionsList[sectionsSelected]!.settings) }}</span>
            </div>
            <div class="collab-doc-pro__sections-row">
              <span class="collab-doc-pro__sections-label">方向</span>
              <span class="collab-doc-pro__sections-value">{{ sectionsList[sectionsSelected]!.settings.orientation === 'landscape' ? '横向' : '纵向' }}</span>
            </div>
            <div class="collab-doc-pro__sections-row">
              <span class="collab-doc-pro__sections-label">分节方式</span>
              <span class="collab-doc-pro__sections-value">{{ sectionsList[sectionsSelected]!.startType }}</span>
            </div>
            <div class="collab-doc-pro__sections-row">
              <span class="collab-doc-pro__sections-label">上 / 下边距</span>
              <span class="collab-doc-pro__sections-value">
                {{ fromTwips(sectionsList[sectionsSelected]!.settings.marginTop, 'inches') }}″ /
                {{ fromTwips(sectionsList[sectionsSelected]!.settings.marginBottom, 'inches') }}″
              </span>
            </div>
            <div class="collab-doc-pro__sections-row">
              <span class="collab-doc-pro__sections-label">左 / 右边距</span>
              <span class="collab-doc-pro__sections-value">
                {{ fromTwips(sectionsList[sectionsSelected]!.settings.marginLeft, 'inches') }}″ /
                {{ fromTwips(sectionsList[sectionsSelected]!.settings.marginRight, 'inches') }}″
              </span>
            </div>
            <div class="collab-doc-pro__sections-row">
              <span class="collab-doc-pro__sections-label">分栏</span>
              <span class="collab-doc-pro__sections-value">{{ sectionsList[sectionsSelected]!.settings.columns }} 栏</span>
            </div>
            <div class="collab-doc-pro__sections-row">
              <span class="collab-doc-pro__sections-label">首页独立</span>
              <span class="collab-doc-pro__sections-value">{{ sectionsList[sectionsSelected]!.titlePg ? '是' : '否' }}</span>
            </div>
            <div class="collab-doc-pro__sections-row">
              <span class="collab-doc-pro__sections-label">块范围</span>
              <span class="collab-doc-pro__sections-value">
                #{{ sectionsList[sectionsSelected]!.firstBlockIndex }} – #{{ sectionsList[sectionsSelected]!.lastBlockIndex }}
              </span>
            </div>
          </div>
        </div>
        <div class="collab-doc-pro__math-actions">
          <button type="button" @click="closeSectionsModal">关闭</button>
        </div>
      </div>
    </div>
    <!-- v0.7.73 — DOC find / replace panel (Word Home > Find / Replace) -->
    <div v-if="findOpen" class="collab-doc-pro__math-bg" @click="closeFindPanel">
      <div
        class="collab-doc-pro__math collab-doc-pro__find"
        data-testid="doc-find-panel"
        @click.stop
      >
        <div class="collab-doc-pro__math-head">查找与替换</div>
        <div class="collab-doc-pro__find-field">
          <label class="collab-doc-pro__find-label">查找内容</label>
          <input
            ref="findQueryInput"
            v-model="findQuery"
            type="text"
            class="collab-doc-pro__find-input"
            data-testid="doc-find-input"
            placeholder="输入要查找的文本"
            @input="onFindQueryInput"
          />
        </div>
        <div class="collab-doc-pro__find-field">
          <label class="collab-doc-pro__find-label">替换为</label>
          <input
            v-model="findReplaceWith"
            type="text"
            class="collab-doc-pro__find-input"
            data-testid="doc-find-replace-input"
            placeholder="（留空则删除）"
          />
        </div>
        <div class="collab-doc-pro__find-opts">
          <label class="collab-doc-pro__find-opt">
            <input v-model="findMatchCase" type="checkbox" data-testid="doc-find-case" @change="onFindOptsChange" />
            区分大小写
          </label>
          <label class="collab-doc-pro__find-opt">
            <input v-model="findWholeWord" type="checkbox" data-testid="doc-find-word" @change="onFindOptsChange" />
            全字匹配
          </label>
        </div>
        <div class="collab-doc-pro__find-status" data-testid="doc-find-status">
          <span v-if="!findQuery">输入关键词开始查找</span>
          <span v-else-if="findMatchesList.length === 0">未找到匹配</span>
          <span v-else>
            第 {{ findCurrentIdx + 1 }} / {{ findMatchesList.length }} 处匹配
          </span>
        </div>
        <div class="collab-doc-pro__find-actions">
          <button type="button" data-testid="doc-find-prev" :disabled="findMatchesList.length === 0" @click="goToPrevMatch">上一个</button>
          <button type="button" data-testid="doc-find-next" :disabled="findMatchesList.length === 0" @click="goToNextMatch">下一个</button>
          <button type="button" data-testid="doc-find-replace" :disabled="findCurrentIdx < 0" @click="doReplaceCurrent">替换</button>
          <button type="button" data-testid="doc-find-replace-all" :disabled="findMatchesList.length === 0" @click="doReplaceAll">全部替换</button>
          <button type="button" data-testid="doc-find-close" @click="closeFindPanel">关闭</button>
        </div>
      </div>
    </div>
    <!-- v0.7.29 — comments side panel -->
    <CollabCommentsPanel
      :doc-id="docId"
      :token="token"
      :anchor="commentAnchor"
      anchor-label="段落选区"
      placeholder="对选中的段落添加评论…"
      @created="onCommentCreated"
      @deleted="onCommentDeleted"
      @loaded="onCommentsLoaded"
    />

    <!-- v0.7.74 — DOC bottom status bar (GenOffice style: page / word count / zoom) -->
    <div class="collab-doc-pro__statusbar" v-if="editor">
      <span class="collab-doc-pro__statusbar-item">页 {{ pageNumber }} / {{ pageCount }}</span>
      <span class="collab-doc-pro__statusbar-sep">·</span>
      <span class="collab-doc-pro__statusbar-item">字数 {{ wordCount }}</span>
      <span class="collab-doc-pro__statusbar-sep">·</span>
      <span class="collab-doc-pro__statusbar-item">{{ connectionLabel }}</span>
      <span class="collab-doc-pro__statusbar-spacer"></span>
      <span class="collab-doc-pro__statusbar-item">{{ protectionEnabled ? '🔒 已保护' : '可编辑' }}</span>
      <span class="collab-doc-pro__statusbar-sep">·</span>
      <button class="collab-doc-pro__statusbar-btn" type="button" data-testid="doc-zoom-out" @click="onZoomOut" title="缩小">−</button>
      <span class="collab-doc-pro__statusbar-zoom">{{ zoomPercent }}%</span>
      <button class="collab-doc-pro__statusbar-btn" type="button" data-testid="doc-zoom-in" @click="onZoomIn" title="放大">＋</button>
    </div>
  </div>
</template>

<script setup lang="ts">
// v0.7.74 — DOC Ribbon 顶部 tab (文件/编辑/审阅/AI) — 桥接到 CollabEditorRibbon
import CollabEditorRibbon from '@/components/collab/CollabEditorRibbon.vue'
import CollabIcon from '@/components/collab/CollabIcon.vue'
import CollabDocThemePanel from '@/components/collab/CollabDocThemePanel.vue'
import CollabDocRuler from '@/components/collab/CollabDocRuler.vue'
type DocRibbonTabId = 'file' | 'home' | 'insert' | 'draw' | 'design' | 'review' | 'view' | 'ai'
const docRibbonTabs: { id: DocRibbonTabId; label: string }[] = [
  { id: 'file',   label: '文件' },
  { id: 'home',   label: '开始' },
  { id: 'insert', label: '插入' },
  { id: 'draw',   label: '绘图' },
  { id: 'design', label: '设计' },
  { id: 'review', label: '审阅' },
  { id: 'view',   label: '视图' },
  { id: 'ai',     label: 'AI'   },
]
const activeTab = ref<DocRibbonTabId>('file')
// v0.7.78 — DOC 8-tab ribbon 扩展（开始/插入/绘图/设计/视图）+ 视图开关
const rulerVisible = ref(false)
const rulerLeftMargin = ref(64)
const rulerRightMargin = ref(64)
const gridlinesVisible = ref(false)
const onZoom100 = () => { zoom.value = 1; applyZoom() }
// 主题（用 8 个固定主题色，DOC 也用与 PPT 相同色板）
type DocThemeId = 'office' | 'ember' | 'indigo' | 'forest' | 'cream' | 'rose' | 'graphite' | 'midnight'
const DOC_THEMES: Record<DocThemeId, { name: string; accent: string; bg: string; fg: string }> = {
  office:   { name: 'Office',   accent: '#5aa8ff', bg: '#ffffff', fg: '#1a1a1a' },
  ember:    { name: 'Ember',    accent: '#f06b3f', bg: '#fffaf6', fg: '#1a1a1a' },
  indigo:   { name: 'Indigo',   accent: '#6366f1', bg: '#f5f5ff', fg: '#1a1a1a' },
  forest:   { name: 'Forest',   accent: '#2f9e44', bg: '#f4faf4', fg: '#1a1a1a' },
  cream:    { name: 'Cream',    accent: '#b58863', bg: '#fdf6e3', fg: '#1a1a1a' },
  rose:     { name: 'Rose',     accent: '#e64980', bg: '#fff0f6', fg: '#1a1a1a' },
  graphite: { name: 'Graphite', accent: '#adb5bd', bg: '#f8f9fa', fg: '#1a1a1a' },
  midnight: { name: 'Midnight', accent: '#7c3aed', bg: '#1e1b4b', fg: '#e9ecef' },
}
const activeDocTheme = ref<DocThemeId>('office')
const onApplyDocTheme = (themeId: DocThemeId) => {
  activeDocTheme.value = themeId
  const theme = DOC_THEMES[themeId]
  const surface = document.querySelector('.collab-doc-pro__surface') as HTMLElement | null
  if (surface) {
    surface.style.setProperty('--doc-theme-accent', theme.accent)
    surface.style.setProperty('--doc-theme-bg', theme.bg)
    surface.style.setProperty('--doc-theme-fg', theme.fg)
    surface.style.background = theme.bg
    surface.style.color = theme.fg
  }
}
// 形状插入（DOC 简易 MVP）
const onInsertShape = (kind: 'rect' | 'ellipse') => {
  const ed = editor.value
  if (!ed) return
  const svg = kind === 'rect'
    ? '<svg xmlns="http://www.w3.org/2000/svg" width="120" height="80"><rect x="2" y="2" width="116" height="76" fill="#5aa8ff" stroke="#3a7bd5" stroke-width="2"/></svg>'
    : '<svg xmlns="http://www.w3.org/2000/svg" width="120" height="80"><ellipse cx="60" cy="40" rx="58" ry="38" fill="#5aa8ff" stroke="#3a7bd5" stroke-width="2"/></svg>'
  ed.chain().focus().insertContent(`<p>${svg}</p>`).run()
}

import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { Editor, EditorContent, Mark } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import { DocTable, DocTableRow, DocTableCell, DocTableHeader } from '@/editor/adapters/docTableExtras'
import { applyTablePreset, toggleRepeatHeaderRows } from '@/editor/adapters/docTableProperties'
import { DocTableHandle } from '@/editor/adapters/docTableHandle'
import Image from '@tiptap/extension-image'
import TaskList from '@tiptap/extension-task-list'
import TaskItem from '@tiptap/extension-task-item'
import Underline from '@tiptap/extension-underline'
import TextAlign from '@tiptap/extension-text-align'
import Highlight from '@tiptap/extension-highlight'
import Color from '@tiptap/extension-color'
import { Collaboration } from '@tiptap/extension-collaboration'
import { CollaborationCursor } from '@tiptap/extension-collaboration-cursor'
import * as Y from 'yjs'
import { useYjsCollabDoc } from '@/composables/useYjsCollabDoc'
import CollabCommentsPanel from '@/components/collab/CollabCommentsPanel.vue'
import {
  openDocx,
  saveDocxBytes,
  saveDocxBytesWithImages,
  patchParagraphText,
  latexToDocxMath,
  docxMathToMathML,
  buildBlankDocxDoc,
  pmDocToSavePlan,
  type DocxAdapterDocument,
  type DocxAdapterParagraph,
} from '@/editor/adapters/docxAdapter'
import {
  downloadCollabDocBytes,
  uploadCollabDocBytes,
} from '@/api/collabDoc'
import { MessagePlugin } from 'tdesign-vue-next'
import { importTextToDocParagraphs, type DocImportParagraph } from '@/editor/adapters/docTextImport'
import CollabAiPolishDialog from './CollabAiPolishDialog.vue'
import { DocInlineMath, DocProtected } from '@/editor/adapters/docNodes'
import { equationBlockJson } from '@/editor/adapters/docEquation'
import { setSelectedColumnWidth, constrainSelectedTableWidth } from '@/editor/adapters/docTableSizing'
import { CommentMark, addCommentToSelection, removeCommentFromDoc } from '@/editor/adapters/docComments'
import { DocPageBreak } from '@/editor/adapters/docPageBreak'
import { collectHeadings, type HeadingRef } from '@/editor/adapters/docHeadings'
import { findMatches, replaceMatch, replaceAllMatches, type FindRange, type FindOptions } from '@/editor/adapters/docFind'
import { moveBlocks } from '@/editor/adapters/docMoveBlock'
import { markdownPasteHtml } from '@/editor/adapters/docMarkdownPaste'
import { applyCase, nextCaseMode, type CaseMode } from '@/editor/adapters/docCaseTransform'
import { clearFormatting, hasFormatting } from '@/editor/adapters/docClearFormat'
import { getDocumentSections, formatSectionSummary, fromTwips, paperLabel, type DocSectionSummary } from '@/editor/adapters/docSections'
import { hashProtectionPassword } from '@/editor/engines/docx-engine/index'
import {
  makeProtectionPatch,
  applyDocProtection,
  validateProtectionPatch,
  PROTECTION_I18N,
  type DocProtectionPatch,
} from '@/editor/adapters/docProtection'
import {
  acceptAllRevisions,
  rejectAllRevisions,
  applyRevisionsBy,
  collectRevisions,
  gotoRevision,
  revisionLabel,
  acceptCurrentRevision,
  rejectCurrentRevision,
} from '@/editor/adapters/docRevisions'
import {
  compareParagraphs,
  summarize,
  blockTexts,
  type CompareEntry,
} from '@/editor/adapters/docCompare'
import type { DocProtection, WriteProtection } from '@/editor/engines/docx-engine'
import type { CollabDocComment } from '@/api/collabDoc'
import type { CommentInfo } from '@/editor/engines/docx-engine'

const props = defineProps<{
  docId: string
  title: string
  token: string
  displayName: string
}>()

const editor = shallowRef<Editor | undefined>(undefined)
const connected = ref(false)
const peers = ref<Array<{ clientId: number; displayName: string; color: string; selection?: { from: number; to: number } | null }>>([])
const error = ref<string | null>(null)
const loading = ref(false)
const loadError = ref<string | null>(null)
const downloading = ref(false)
const uploading = ref(false)
const saveLabel = ref('未修改')
const saveError = ref<string | null>(null)
const loadRecovery = ref<string | null>(null)
const aiOpen = ref(false)
const aiAnchor = ref({ x: 0, y: 0 })
const aiOriginal = ref('')
let aiTargetIndex: number | null = null
const historyOpen = ref(false)
const zoom = ref(1)
const zoomPercent = computed(() => Math.round(zoom.value * 100))
const pageNumber = ref(1)
const wordCount = ref(0)
const pageCount = ref(1)
const onZoomIn = () => { zoom.value = Math.min(2, +(zoom.value + 0.1).toFixed(2)); applyZoom() }
const onZoomOut = () => { zoom.value = Math.max(0.5, +(zoom.value - 0.1).toFixed(2)); applyZoom() }
const applyZoom = () => {
  const surface = document.querySelector('.collab-doc-pro__surface')
  if (surface) (surface as HTMLElement).style.zoom = String(zoom.value)
}
const refreshStats = () => {
  const ed = editor.value
  if (!ed) return
  const txt = ed.state.doc.textContent
  wordCount.value = txt.replace(/\s+/g, ' ').trim().split(' ').filter(Boolean).length
  pageCount.value = Math.max(1, Math.ceil(ed.state.doc.content.size / 800))
}

// v0.7.71 — DOC heading outline (mirrors genoffice NavPane.tsx, Vue port).
// collectHeadings reads the live ProseMirror doc; outlineTick bumps on every
// editor update so the list re-renders even when the doc ref would otherwise
// short-circuit the computed (Pinia/Vue reactivity on `state` is shallow).
const outlineOpen = ref(false)
const outlineTick = ref(0)
const outlineList = computed<HeadingRef[]>(() => {
  outlineTick.value
  if (!editor.value) return []
  return collectHeadings(editor.value.state.doc)
})
const onToggleOutline = () => {
  outlineOpen.value = !outlineOpen.value
  outlineTick.value++
}
const onOutlineJump = (h: HeadingRef) => {
  if (!editor.value) return
  const dom = editor.value.view.nodeDOM(h.pos) as HTMLElement | null
  dom?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  // Place caret inside the heading so the next keystroke lands there.
  editor.value.commands.focus(h.pos + 1)
}

// v0.7.72 — DOC page-setup / multi-section panel.
// Read-only preview of all sections in document order (paper, orientation,
// margins, columns). Uses getDocumentSections on the latest parsed doc.
const sectionsOpen = ref(false)
const sectionsList = ref<DocSectionSummary[]>([])
const sectionsSelected = ref(0)
const openSectionsModal = () => {
  if (!doc) {
    MessagePlugin.warning('文档未加载')
    return
  }
  sectionsList.value = getDocumentSections(doc as any)
  sectionsSelected.value = 0
  sectionsOpen.value = true
}
const closeSectionsModal = () => {
  sectionsOpen.value = false
}
const onSelectSection = (idx: number) => {
  sectionsSelected.value = idx
}

// v0.7.73 — DOC find / replace (mirrors genoffice FindPanel.tsx; UI inlined here to keep change minimal).
// Pure helpers live in adapters/docFind.ts; this section only wires them into
// the editor view (selection scroll + count display + replace dispatch).
const findOpen = ref(false)
const findQuery = ref('')
const findReplaceWith = ref('')
const findMatchCase = ref(false)
const findWholeWord = ref(false)
const findMatchesList = ref<FindRange[]>([])
const findCurrentIdx = ref(-1)
const findOpts = computed<FindOptions>(() => ({
  matchCase: findMatchCase.value,
  wholeWord: findWholeWord.value,
}))
const refreshFindMatches = () => {
  if (!editor.value) {
    findMatchesList.value = []
    findCurrentIdx.value = -1
    return
  }
  findMatchesList.value = findMatches(editor.value, findQuery.value, findOpts.value)
  if (findMatchesList.value.length > 0) {
    findCurrentIdx.value = Math.min(findCurrentIdx.value, findMatchesList.value.length - 1)
    if (findCurrentIdx.value < 0) findCurrentIdx.value = 0
  } else {
    findCurrentIdx.value = -1
  }
}
const openFindPanel = () => {
  findOpen.value = !findOpen.value
  if (findOpen.value) refreshFindMatches()
}
const closeFindPanel = () => {
  findOpen.value = false
}
const onFindQueryInput = () => {
  findCurrentIdx.value = -1
  refreshFindMatches()
}
const onFindOptsChange = () => {
  refreshFindMatches()
}
const scrollToMatch = (idx: number) => {
  const r = findMatchesList.value[idx]
  if (!r || !editor.value) return
  editor.value.commands.setTextSelection(r.from)
  const dom = editor.value.view.nodeDOM(r.from) as HTMLElement | null
  dom?.scrollIntoView({ behavior: 'smooth', block: 'center' })
}
const goToNextMatch = () => {
  if (findMatchesList.value.length === 0) return
  findCurrentIdx.value = (findCurrentIdx.value + 1) % findMatchesList.value.length
  scrollToMatch(findCurrentIdx.value)
}
const goToPrevMatch = () => {
  if (findMatchesList.value.length === 0) return
  findCurrentIdx.value =
    (findCurrentIdx.value - 1 + findMatchesList.value.length) % findMatchesList.value.length
  scrollToMatch(findCurrentIdx.value)
}
const doReplaceCurrent = () => {
  if (!editor.value || findCurrentIdx.value < 0) return
  const r = findMatchesList.value[findCurrentIdx.value]
  if (!r) return
  replaceMatch(editor.value, r, findReplaceWith.value)
  refreshFindMatches()
}
const doReplaceAll = () => {
  if (!editor.value || findMatchesList.value.length === 0) return
  replaceAllMatches(editor.value, findMatchesList.value, findReplaceWith.value)
  refreshFindMatches()
}

// v0.7.74 — Word Alt+Shift+Up / Alt+Shift+Down: move selected block past its neighbor.
const onMoveBlock = (dir: -1 | 1) => {
  if (!editor.value) return
  moveBlocks(editor.value, dir)
}

// v0.7.75 — Word Shift+F3: cycle case on the current selection (lower → UPPER → Title).
const onCycleCase = () => {
  if (!editor.value) return
  const sel = editor.value.state.selection
  if (sel.from === sel.to) return
  // Extract the selection text via ProseMirror textBetween.
  const text = editor.value.state.doc.textBetween(sel.from, sel.to, '\n', '\n')
  const mode = nextCaseMode(text) as CaseMode
  applyCase(editor.value, mode)
}

// v0.7.78 — Word Ctrl+Space: strip all character-level formatting from the selection.
const canClearFormat = computed(() => (editor.value ? hasFormatting(editor.value) : false))
const onClearFormat = () => {
  if (!editor.value) return
  clearFormatting(editor.value)
}

// v0.7.42 — Math formula dialog (LaTeX → MathML via docxAdapter).
// Uses browser-native MathML for preview (Firefox / Safari / Edge);
// Chrome shows source until a KaTeX renderer is wired in later.
const mathOpen = ref(false)
const mathLatex = ref('')
const mathError = ref<string | null>(null)
const mathPreviewHtml = computed(() => {
  const latex = mathLatex.value.trim()
  if (!latex) return '<em style="color:#999">在上方输入 LaTeX 以预览</em>'
  const omml = latexToDocxMath(latex)
  if (!omml) {
    mathError.value = '无法解析 LaTeX 语法'
    return ''
  }
  mathError.value = null
  const wrapped = '<m:oMath xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math">' + omml + '</m:oMath>'
  const mathml = docxMathToMathML(wrapped)
  if (!mathml) return '<em style="color:#999">MathML 渲染不可用</em>'
  return mathml
})
// v0.7.63 — Word-style page break (Ctrl+Enter equivalent)
// v0.7.66 — DOC page header/footer
const hfOpen = ref(false)

// v0.7.67 — DOC document protection (Word Review > Protect Document)
const protectOpen = ref(false)
const protectPatch = ref<DocProtectionPatch>(makeProtectionPatch(null))
const currentProtection = ref<DocProtection | null>(null)
const protectionEnabled = computed(() => currentProtection.value?.enforced === true)
const protectErrorText = computed(() => PROTECTION_I18N[protectPatch.value.error] || protectPatch.value.error)

// v0.7.68 — DOC track changes
const revisionsOpen = ref(false)
const revisionTick = ref(0) // bump to recompute revisions on selection update
// v0.7.106 — track-changes recording mode. When on, typing inserts text
// wrapped in an `ins` mark; range replacements leave behind del marks on the
// surviving text (mirroring Word's behavior).
const trackChangesOn = ref(false)
let trackChangeSeq = 9000 // monotonically increasing revision id
const onToggleTrackChanges = () => {
  trackChangesOn.value = !trackChangesOn.value
}
const revisionCount = computed(() => {
  revisionTick.value
  return editor.value ? collectRevisions(editor.value.state.doc).length : 0
})
interface RevisionSnippet {
  from: number
  to: number
  kind: ReturnType<typeof collectRevisions>[number]['kind']
  snippet: string
  /** v0.7.106 — ISO timestamp from the mark's `date` attr (undefined when missing). */
  date?: string
}
const revisions = computed<RevisionSnippet[]>(() => {
  revisionTick.value
  if (!editor.value) return []
  return collectRevisions(editor.value.state.doc).map((r) => ({
    from: r.from,
    to: r.to,
    kind: r.kind,
    snippet: editor.value!.state.doc.textBetween(r.from, r.to, ' ', ' ').trim().slice(0, 40),
    ...(r.date ? { date: r.date } : {}),
  }))
})
const revisionsByAuthor = computed(() => {
  if (!editor.value) return []
  const groups = new Map<string, { author: string; items: RevisionSnippet[]; count: number }>()
  for (const r of collectRevisions(editor.value.state.doc)) {
    const snippet: RevisionSnippet = {
      from: r.from,
      to: r.to,
      kind: r.kind,
      snippet: editor.value!.state.doc.textBetween(r.from, r.to, ' ', ' ').trim().slice(0, 40),
      ...(r.date ? { date: r.date } : {}),
    }
    const existing = groups.get(r.author)
    if (existing) {
      existing.items.push(snippet)
      existing.count++
    } else {
      groups.set(r.author, { author: r.author, items: [snippet], count: 1 })
    }
  }
  return Array.from(groups.values())
})
const openRevisionsPanel = () => {
  revisionsOpen.value = true
}
const onAcceptAll = () => {
  if (!editor.value) return
  acceptAllRevisions(editor.value)
  revisionTick.value++
  scheduleSave()
}
const onRejectAll = () => {
  if (!editor.value) return
  rejectAllRevisions(editor.value)
  revisionTick.value++
  scheduleSave()
}
const onAcceptAuthor = (author: string) => {
  if (!editor.value) return
  applyRevisionsBy(editor.value, author, 'accept')
  revisionTick.value++
  scheduleSave()
}
const onRejectAuthor = (author: string) => {
  if (!editor.value) return
  applyRevisionsBy(editor.value, author, 'reject')
  revisionTick.value++
  scheduleSave()
}
const onRevisionGoto = (rev: RevisionSnippet) => {
  if (!editor.value) return
  editor.value.commands.setTextSelection({ from: rev.from, to: rev.to })
  revisionsOpen.value = false
}
// v0.7.103 — per-revision viewer: goto + accept/reject a single revision.
const onRevisionItemGoto = (rev: RevisionSnippet) => {
  if (!editor.value) return
  editor.value.commands.setTextSelection({ from: rev.from, to: rev.to })
  editor.value.commands.focus()
}
const onRevisionItemAccept = (rev: RevisionSnippet) => {
  if (!editor.value) return
  onRevisionItemGoto(rev)
  if (acceptCurrentRevision(editor.value)) {
    revisionTick.value++
    scheduleSave()
  }
}
const onRevisionItemReject = (rev: RevisionSnippet) => {
  if (!editor.value) return
  onRevisionItemGoto(rev)
  if (rejectCurrentRevision(editor.value)) {
    revisionTick.value++
    scheduleSave()
  }
}

// v0.7.106 — friendly relative timestamp for the revisions panel. Falls back
// to the raw ISO string when the timestamp is older than 24 h or in the future.
const formatRevDate = (iso: string): string => {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return iso
  const now = Date.now()
  const delta = (now - t) / 1000
  if (delta < 0) return iso
  if (delta < 60) return '刚刚'
  if (delta < 3600) return `${Math.floor(delta / 60)} 分钟前`
  if (delta < 86400) return `${Math.floor(delta / 3600)} 小时前`
  const d = new Date(t)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// v0.7.69 — DOC compare (Word Review > Compare)
const compareOpen = ref(false)
const compareOther = ref<{ name: string; entries: CompareEntry[] } | null>(null)
const compareSummary = computed(() =>
  compareOther.value ? summarize(compareOther.value.entries) : { added: 0, removed: 0, changed: 0 },
)
interface CompareRow {
  kind: 'same' | 'entry'
  count?: number
  entry?: CompareEntry
}
const compareRows = computed<CompareRow[]>(() => {
  if (!compareOther.value) return []
  const rows: CompareRow[] = []
  let sameRun = 0
  for (const entry of compareOther.value.entries) {
    if (entry.kind === 'same') {
      sameRun++
      continue
    }
    if (sameRun > 0) {
      rows.push({ kind: 'same', count: sameRun })
      sameRun = 0
    }
    rows.push({ kind: 'entry', entry })
  }
  if (sameRun > 0) rows.push({ kind: 'same', count: sameRun })
  return rows
})
const openCompareModal = () => {
  compareOther.value = null
  compareOpen.value = true
}
const onCompareFileSelected = async (e: Event) => {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    const bytes = new Uint8Array(await file.arrayBuffer())
    const otherDoc = await openDocx(bytes)
    const otherTexts = blockTexts(otherDoc.parsed.blocks)
    const currentTexts = doc ? blockTexts(doc.parsed.blocks) : []
    const entries = compareParagraphs(currentTexts, otherTexts)
    compareOther.value = { name: file.name, entries }
  } catch (err: any) {
    MessagePlugin.error(`解析失败：${err?.message || err}`)
  } finally {
    if (input) input.value = ''
  }
}
const headerTextInput = ref('')
const footerTextInput = ref('')
const footerPageNumberInput = ref(false)
const openHfModal = () => {
  headerTextInput.value = pendingHeader.value?.text ?? ''
  footerTextInput.value = pendingFooter.value?.text ?? ''
  footerPageNumberInput.value = pendingFooter.value?.pageNumber ?? false
  hfOpen.value = true
}
const pendingHeader = ref<{ text: string } | null>(null)
const pendingFooter = ref<{ text: string; pageNumber?: boolean } | null>(null)
// v0.7.67 — document protection pending state (applied on next save)
const pendingProtection = ref<DocProtection | null>(null)
const pendingWriteProtection = ref<WriteProtection | null>(null)
const onHfClear = () => {
  pendingHeader.value = null
  pendingFooter.value = null
  hfOpen.value = false
  scheduleSave()
}
const onHfSave = () => {
  pendingHeader.value = headerTextInput.value.trim() ? { text: headerTextInput.value } : null
  pendingFooter.value = footerTextInput.value.trim() || footerPageNumberInput.value
    ? { text: footerTextInput.value, pageNumber: footerPageNumberInput.value }
    : null
  hfOpen.value = false
  scheduleSave()
}

const openProtectModal = () => {
  protectPatch.value = makeProtectionPatch(currentProtection.value)
  protectOpen.value = true
}
const onProtectEnabledChange = (enabled: boolean) => {
  protectPatch.value.enabled = enabled
  protectPatch.value.error = ''
}
const onProtectClear = () => {
  protectPatch.value.enabled = false
  protectPatch.value.error = ''
  onProtectSave()
}
const onProtectSave = async () => {
  const validationError = validateProtectionPatch(protectPatch.value)
  if (validationError) {
    protectPatch.value.error = validationError
    return
  }
  protectPatch.value.busy = true
  try {
    const { protection, error } = await applyDocProtection(
      currentProtection.value,
      protectPatch.value,
    )
    if (error) {
      protectPatch.value.error = error
      return
    }
    currentProtection.value = protection
    pendingProtection.value = protection
    protectOpen.value = false
    scheduleSave()
  } finally {
    protectPatch.value.busy = false
  }
}

const onInsertPageBreak = () => {
  const ed = editor.value
  if (!ed) return
  const { $from } = ed.state.selection
  if (!$from.parent.isTextblock) return
  const tr = ed.state.tr.setNodeMarkup($from.before($from.depth), undefined, {
    ...$from.parent.attrs,
    pageBreakBefore: true,
  })
  ed.view.dispatch(tr.scrollIntoView())
}

const onOpenMath = () => {
  mathLatex.value = ''
  mathError.value = null
  mathOpen.value = true
}
const onInsertMath = () => {
  const latex = mathLatex.value.trim()
  if (!latex) {
    mathError.value = '请输入 LaTeX'
    return
  }
  let node: ReturnType<typeof equationBlockJson>
  try {
    node = equationBlockJson(latex)
  } catch {
    mathError.value = '无法解析 LaTeX 语法'
    return
  }
  if (editor.value) {
    // v0.7.49 — structured docProtected node: genXml carries the OMML
    // paragraph so the save path can round-trip the formula faithfully.
    const idx = findDocxIndexAt(editor.value)
    if (idx != null) node.attrs!.docxIndex = idx
    editor.value.chain().focus().insertContent(node).run()
    mathOpen.value = false
    scheduleSave()
  }
}

/** docxIndex of the block containing the current selection (formula anchor). */
const findDocxIndexAt = (ed: import('@tiptap/core').Editor, pos?: number): number | null => {
  const from = pos ?? ed.state.selection.from
  const resolved = ed.state.doc.resolve(from)
  for (let depth = resolved.depth; depth >= 0; depth--) {
    const idx = parseDocxIndex(resolved.node(depth))
    if (idx != null) return idx
  }
  return null
}
const versions = ref<Array<{ version: number; size_bytes: number; created_at: string }>>([])

let handle: ReturnType<typeof useYjsCollabDoc> | null = null
let ydoc: Y.Doc | null = null
let doc: DocxAdapterDocument | null = null
let saveTimer: ReturnType<typeof setTimeout> | null = null
let suppressObserver = false
let patchedMap: Map<number, string> = new Map()

const connectionLabel = computed(() => {
  if (saveError.value) return `保存错误：${saveError.value}`
  if (connected.value) return '已连接'
  return '连接中...'
})

const kindLabel = computed(() => 'Word 文档 (.docx)')

const savetagClass = computed(() => ({
  dirty: saveLabel.value === '有未保存的修改',
  saving: saveLabel.value === '保存中...',
}))

const initialOf = (name: string) => (name || '?').trim().slice(0, 1).toUpperCase()

// v0.7.38 — remote selection broadcast
const remoteSelections = ref<Array<{
  clientId: number
  displayName: string
  color: string
  selection?: { from: number; to: number } | null
}>>([])

const remoteSelectionFor = (clientId: number) => {
  return remoteSelections.value.find((p) => p.clientId === clientId)?.selection || null
}

const peerTitle = (p: { displayName: string; clientId: number; color: string }) => {
  const sel = remoteSelectionFor(p.clientId)
  const range = sel ? ` — 选区 ${sel.from}–${sel.to}` : ''
  return `${p.displayName}${range}`
}

const downloadAsUint8Array = async (): Promise<Uint8Array> => {
  const blob = await downloadCollabDocBytes(props.docId)
  const buffer = await blob.arrayBuffer()
  return new Uint8Array(buffer)
}

const onDownload = async () => {
  if (!doc) return
  if (!doc) return
  const curDoc = doc
  downloading.value = true
  try {
    const patched = Array.from(patchedMap.entries()).map(([docxIndex, xml]) => ({
      docxIndex, xml, text: curDoc.paragraphs[docxIndex]?.text || '',
    }))
    const bytes = await saveDocxBytes(curDoc, patched)
    const ab = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer
    const blob = new Blob([ab], {
      type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${props.title || 'collab-doc'}.docx`
    a.click()
    URL.revokeObjectURL(a.href)
    MessagePlugin.success('已下载 .docx')
  } catch (e: any) {
    MessagePlugin.error(`下载失败：${e?.message || e}`)
  } finally {
    downloading.value = false
  }
}

const onForceSave = () => {
  flushSave(true)
}

// v0.7.58 — Tencent Docs compatibility: .txt/.md/.docx import.
const importParagraphsToContent = (paragraphs: DocImportParagraph[]) => {
  const nodes: any[] = []
  for (let i = 0; i < paragraphs.length; i++) {
    const p = paragraphs[i]
    const textNode = p.text ? [{ type: 'text', text: p.text }] : []
    if (p.kind === 'heading') {
      nodes.push({
        type: 'heading',
        attrs: { level: p.level ?? 1, 'data-docx-index': i },
        content: textNode,
      })
    } else if (p.kind === 'listItem') {
      nodes.push({
        type: 'bulletList',
        content: [{
          type: 'listItem',
          attrs: { 'data-docx-index': i },
          content: [{ type: 'paragraph', content: textNode }],
        }],
      })
    } else {
      nodes.push({
        type: 'paragraph',
        attrs: { 'data-docx-index': i },
        content: textNode,
      })
    }
  }
  if (nodes.length === 0) {
    nodes.push({ type: 'paragraph', attrs: { 'data-docx-index': 0 }, content: [] })
  }
  return { type: 'doc', content: nodes }
}

const triggerDocUpload = () => docFileInput.value?.click()

const onUploadDocFile = async (e: Event) => {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  uploading.value = true
  try {
    const name = file.name.toLowerCase()
    if (name.endsWith('.docx')) {
      const bytes = new Uint8Array(await file.arrayBuffer())
      doc = await openDocx(bytes)
      currentProtection.value = doc.parsed.protection ?? null
      pendingProtection.value = currentProtection.value
      if (editor.value) {
        editor.value.commands.setContent(paragraphsToContent(doc.paragraphs), false)
      }
      patchedMap.clear()
      await uploadCollabDocBytes(props.docId, bytes, file.name)
      saveLabel.value = '已上传'
      MessagePlugin.success(`已上传 ${file.name}`)
      return
    }
    const text = await file.text()
    const paragraphs = importTextToDocParagraphs(text)
    doc = await buildBlankDocxDoc(paragraphs)
    if (editor.value) {
      editor.value.commands.setContent(importParagraphsToContent(paragraphs), false)
    }
    patchedMap.clear()
    await flushSave(true)
    saveLabel.value = '已上传'
    MessagePlugin.success(`已导入 ${file.name}`)
  } catch (err: any) {
    MessagePlugin.error(`上传失败：${err?.message || err}`)
  } finally {
    uploading.value = false
    if (input) input.value = ''
  }
}

const isMarkActive = (name: string): boolean => {
  if (!editor.value) return false
  return editor.value.isActive(name)
}
const isNodeActive = (name: string, attrs?: Record<string, unknown>): boolean => {
  if (!editor.value) return false
  return editor.value.isActive(name, attrs)
}
const runMark = (name: string) => {
  if (!editor.value) return
  const chain = editor.value.chain().focus()
  if (name === 'bold') chain.toggleBold().run()
  else if (name === 'italic') chain.toggleItalic().run()
  else if (name === 'strike') chain.toggleStrike().run()
  else if (name === 'code') chain.toggleCode().run()
  else if (name === 'link') chain.toggleLink({ href: '' }).run()
  else chain.run()
}
const runNode = (cmd: string) => {
  if (!editor.value) return
  const chain = editor.value.chain().focus()
  if (cmd === 'toggleBulletList') chain.toggleBulletList().run()
  else if (cmd === 'toggleOrderedList') chain.toggleOrderedList().run()
  else if (cmd === 'toggleBlockquote') chain.toggleBlockquote().run()
  else if (cmd === 'toggleCodeBlock') chain.toggleCodeBlock().run()
  else if (cmd === 'undo') chain.undo().run()
  else if (cmd === 'redo') chain.redo().run()
  else chain.run()
}
const runHeading = (level: 1 | 2 | 3) => {
  if (!editor.value) return
  editor.value.chain().focus().toggleHeading({ level }).run()
}
const onSetLink = () => {
  if (!editor.value) return
  const prev = editor.value.getAttributes('link').href as string | undefined
  const url = window.prompt('链接地址（留空取消）', prev || 'https://')
  if (url === null) return
  if (url === '') {
    editor.value.chain().focus().extendMarkRange('link').unsetLink().run()
    return
  }
  editor.value.chain().focus().extendMarkRange('link').setLink({ href: url }).run()
}

const onInsertTable = () => {
  if (!editor.value) return
  const rowsStr = window.prompt('行数（含表头，默认 3）', '3')
  const colsStr = window.prompt('列数（默认 3）', '3')
  if (rowsStr === null || colsStr === null) return
  const rows = Math.max(1, Math.min(20, Number(rowsStr) || 3))
  const cols = Math.max(1, Math.min(10, Number(colsStr) || 3))
  editor.value
    .chain()
    .focus()
    .insertTable({ rows, cols, withHeaderRow: true })
    .run()
}

// v0.7.53 — resize the selected columns (px) and redistribute the grid.
const onSetColumnWidth = () => {
  if (!editor.value) return
  const widthStr = window.prompt('列宽 (px，留空取消)', '120')
  if (widthStr === null) return
  const width = Math.max(40, Math.min(600, Number(widthStr) || 120))
  const maxWidth = Math.max(400, (editor.value.view.dom as HTMLElement | null)?.clientWidth ?? 800)
  editor.value.chain().focus().command(({ state, dispatch }) =>
    setSelectedColumnWidth(width, maxWidth)(state, dispatch),
  ).run()
  scheduleSave()
}

// v0.7.54 — apply a Word-style visual preset (header fill + banded rows + borders).
const onApplyTablePreset = () => {
  if (!editor.value) return
  const choice = window.prompt('表格样式: 1=蓝 2=灰 3=无边框 (留空取消)', '1')
  if (choice === null) return
  const presets: Record<string, { headerFill: string | null; band1Fill: string | null; band2Fill: string | null; borderColor: string }> = {
    '1': { headerFill: 'D9E2F3', band1Fill: 'F2F6FC', band2Fill: 'FFFFFF', borderColor: '8EAADB' },
    '2': { headerFill: 'E7E6E6', band1Fill: 'F7F7F7', band2Fill: 'FFFFFF', borderColor: 'BFBFBF' },
    '3': { headerFill: null, band1Fill: null, band2Fill: null, borderColor: 'FFFFFF' },
  }
  const preset = presets[choice.trim()]
  if (!preset) return
  editor.value.chain().focus().command(({ state, dispatch }) =>
    applyTablePreset(preset)(state, dispatch),
  ).run()
  scheduleSave()
}

// v0.7.54 — toggle the selected leading row(s) as repeating table headers.
const onToggleRepeatHeader = () => {
  if (!editor.value) return
  editor.value.chain().focus().command(({ state, dispatch }) =>
    toggleRepeatHeaderRows()(state, dispatch),
  ).run()
  scheduleSave()
}

// --- v0.7.29 — comments anchor (set when the user selects a range) ---
const commentAnchor = ref<{ type: 'doc' | 'slide' | 'sheet'; ref: string } | null>(null)

// v0.7.49 — attach the comment mark to the selected range once the thread
// exists on the backend (the mark carries the numeric comment id).
const onCommentCreated = (comment: CollabDocComment) => {
  if (!editor.value) return
  addCommentToSelection(editor.value, String(comment.id))
  scheduleSave()
}

// v0.7.52 — strip the comment mark once the thread is deleted on the backend.
const onCommentDeleted = (comment: CollabDocComment) => {
  if (!editor.value) return
  removeCommentFromDoc(editor.value, String(comment.id))
  scheduleSave()
}

// v0.7.56 — cache the REST comment threads so the save path can regenerate
// word/comments.xml from them (bodies + authors + replies + resolved flags).
let cachedComments: CollabDocComment[] = []
const onCommentsLoaded = (comments: CollabDocComment[]) => {
  cachedComments = comments
}

/** CollabDocComment[] -> engine CommentInfo[] for the .docx save. */
const commentsToCommentInfo = (comments: CollabDocComment[]): CommentInfo[] => {
  return comments.map((c) => ({
    id: String(c.id),
    author: c.author_name,
    text: c.body,
    date: c.created_at,
    parentId: c.parent_id != null ? String(c.parent_id) : undefined,
    done: c.resolved || undefined,
  }))
}

const onInsertImageUrl = () => {
  if (!editor.value) return
  const url = window.prompt('图片 URL（留空取消）', 'https://')
  if (!url) return
  editor.value.chain().focus().setImage({ src: url }).run()
}

const onInsertImageFile = () => {
  fileImageInput.value?.click()
}

const fileImageInput = ref<HTMLInputElement | null>(null)
const docFileInput = ref<HTMLInputElement | null>(null)
const onImageFileChosen = async (e: Event) => {
  if (!editor.value) return
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  if (!file.type.startsWith('image/')) {
    MessagePlugin.error('请选择图片文件 (PNG/JPEG/GIF/WebP)')
    return
  }
  // Convert to dataURL (TipTap image extension accepts it; the
  // saveDocxBytesWithImages flow will then write the bytes into the .docx).
  const reader = new FileReader()
  reader.onload = () => {
    const dataUrl = String(reader.result || '')
    if (!dataUrl.startsWith('data:')) return
    const img = new window.Image()
    img.onload = () => {
      editor.value!
        .chain()
        .focus()
        .setImage({ src: dataUrl, alt: file.name })
        .run()
    }
    img.src = dataUrl
  }
  reader.readAsDataURL(file)
  if (fileImageInput.value) fileImageInput.value.value = ''
}

const onAlign = (align: 'left' | 'center' | 'right') => {
  if (!editor.value) return
  ;(editor.value.chain().focus() as any)[`setTextAlign`](align).run()
}

const onToggleHistory = async () => {
  historyOpen.value = !historyOpen.value
  if (historyOpen.value) await loadVersions()
}

const loadVersions = async () => {
  try {
    const r = await fetch(`/api/v1/collaborative-docs/${encodeURIComponent(props.docId)}/files`, {
      headers: { Authorization: `Bearer ${props.token}` },
    })
    if (!r.ok) return
    const json = await r.json()
    versions.value = (json.data || []) as Array<{ version: number; size_bytes: number; created_at: string }>
  } catch {
    versions.value = []
  }
}

const formatBytes = (n: number) => {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

const formatTime = (s: string) => (s ? new Date(s).toLocaleString() : '—')

const onDownloadVersion = async (v: number) => {
  try {
    const r = await fetch(`/api/v1/collaborative-docs/${encodeURIComponent(props.docId)}/download/${v}`, {
      headers: { Authorization: `Bearer ${props.token}` },
    })
    if (!r.ok) throw new Error(`status ${r.status}`)
    const blob = await r.blob()
    const ab = await blob.arrayBuffer()
    const buf = new Uint8Array(ab)
    const url = URL.createObjectURL(new Blob([buf], {
      type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    }))
    const a = document.createElement('a')
    a.href = url
    a.download = `${props.title || 'collab-doc'}-v${v}.docx`
    a.click()
    URL.revokeObjectURL(url)
  } catch (e: any) {
    MessagePlugin.error(`下载失败：${e?.message || e}`)
  }
}

const onRestoreVersion = async (v: number) => {
  if (!confirm(`确认将文档恢复到 v${v}？当前未保存的修改会丢失。`)) return
  try {
    const r = await fetch(`/api/v1/collaborative-docs/${encodeURIComponent(props.docId)}/download/${v}`, {
      headers: { Authorization: `Bearer ${props.token}` },
    })
    if (!r.ok) throw new Error(`status ${r.status}`)
    const blob = await r.blob()
    const bytes = new Uint8Array(await blob.arrayBuffer())
    doc = await openDocx(bytes)
    if (editor.value) {
      editor.value.commands.setContent(paragraphsToContent(doc.paragraphs), false)
    }
    patchedMap.clear()
    MessagePlugin.success(`已恢复到 v${v}`)
    scheduleSave()
  } catch (e: any) {
    MessagePlugin.error(`恢复失败：${e?.message || e}`)
  }
}

const refreshAiSelection = () => {
  // Capture the current TipTap selection's text + position so the AI popover
  // can show the selected paragraph and we know which block.docxIndex to
  // patch when the user accepts.
  if (!editor.value) {
    aiOriginal.value = ''
    return
  }
  const { from, to } = editor.value.state.selection
  if (from === to) {
    aiOriginal.value = ''
    aiTargetIndex = null
    return
  }
  const text = editor.value.state.doc.textBetween(from, to, '\n')
  if (!text.trim()) {
    aiOriginal.value = ''
    aiTargetIndex = null
    return
  }
  aiOriginal.value = text
  // Walk back to find the closest paragraph's docx-index attribute.
  const $from = editor.value.state.doc.resolve(from)
  for (let d = $from.depth; d >= 0; d--) {
    const node = $from.node(d)
    if (node?.type?.name === 'paragraph' || node?.type?.name === 'heading') {
      const idx = node.attrs?.['data-docx-index']
      aiTargetIndex = typeof idx === 'number' ? idx : idx ? Number(idx) : null
      break
    }
  }
}

const onOpenAi = () => {
  refreshAiSelection()
  if (!aiOriginal.value) {
    MessagePlugin.warning('请先在文档中选中要润色的段落')
    return
  }
  aiAnchor.value = { x: window.innerWidth / 2 - 240, y: 120 }
  aiOpen.value = true
}

const onAcceptAi = (replacement: string) => {
  aiOpen.value = false
  if (!editor.value || aiTargetIndex == null || !doc) return
  // Apply the replacement to the matching docx-engine block and re-sync
  // the editor's underlying paragraph via editor.commands.
  const targetIdx: number = aiTargetIndex
  patchedMap.set(targetIdx, replacement)
  doc.paragraphs[targetIdx].text = replacement
  // Replace selection text in TipTap.
  const ed = editor.value
  const { from, to } = ed.state.selection
  ed.chain().focus().insertContentAt({ from, to }, replacement).run()
  scheduleSave()
}

const setup = async () => {
  if (!props.docId || !props.token) return
  handle = useYjsCollabDoc({ docId: props.docId, token: props.token, displayName: props.displayName })
  ydoc = handle.ydoc
  connected.value = Boolean(handle.connected.value)
  peers.value = (handle.peers.value ?? []) as Array<{ clientId: number; displayName: string; color: string }>
  error.value = (handle.error.value ?? null) as string | null
  watch(handle.connected, (v) => (connected.value = Boolean(v)))
  watch(handle.peers, (v) => (peers.value = (v ?? []) as Array<{ clientId: number; displayName: string; color: string }>))
  // v0.7.38 — remote selection range awareness (DOC per-paragraph).
  if (handle.remoteSelections) {
    remoteSelections.value = handle.remoteSelections.value as any
    watch(handle.remoteSelections, (v) => (remoteSelections.value = (v ?? []) as any))
  }
  watch(handle.error, (v) => (error.value = (v ?? null) as string | null))

  loading.value = true
  loadError.value = null
  loadRecovery.value = null
  try {
    let bytes: Uint8Array
    try {
      bytes = await downloadAsUint8Array()
    } catch (e: any) {
      // No bytes uploaded yet → start from a blank docx rendered as empty.
      // We deliberately keep the user in the editor so they can type, and
      // the first save will upload a real .docx package.
      doc = null
      loading.value = false
      initEditor([])
      return
    }
    try {
      doc = await openDocx(bytes)
    } catch (parseError: any) {
      // Historical seed files can be truncated or contain malformed XML.
      // Keep the document usable by replacing only the in-memory editor
      // model with the engine's valid blank package; the next explicit save
      // persists a legal .docx instead of leaving the user on a dead error
      // screen. User-uploaded invalid files still fail in onUploadDocFile.
      doc = await buildBlankDocxDoc([])
      loadRecovery.value = `原文档解析失败，已恢复为空白文档（${parseError?.message || '格式错误'}）`
      saveLabel.value = '已恢复，待保存'
    }
    currentProtection.value = doc.parsed.protection ?? null
    pendingProtection.value = currentProtection.value
    initEditor(doc.paragraphs)
  } catch (e: any) {
    loadError.value = e?.message || String(e)
  } finally {
    loading.value = false
  }
}

const initEditor = (paragraphs: DocxAdapterParagraph[]) => {
  editor.value = new Editor({
    extensions: [
      StarterKit.configure({ history: false }),
      DocPageBreak,
      DocInlineMath,
      DocProtected,
      CommentMark,
      // v0.7.103 — register ins / del marks so docRevisions collectRevisions can
      // detect tracked-change runs and accept/reject can operate on them.
      Mark.create({
        name: 'ins', inclusive: false,
        addAttributes: () => ({ author: { default: 'admin' }, date: { default: null } }),
        renderHTML: ({ HTMLAttributes }) => ['ins', { 'data-author': HTMLAttributes.author }, 0],
      }),
      Mark.create({
        name: 'del', inclusive: false,
        addAttributes: () => ({ author: { default: 'admin' }, date: { default: null } }),
        renderHTML: ({ HTMLAttributes }) => ['del', { 'data-author': HTMLAttributes.author }, 0],
      }),
      Link,
      Underline,
      Highlight.configure({ multicolor: true }),
      Color,
      TextAlign.configure({ types: ['heading', 'paragraph'] }),
      DocTable.configure({ resizable: true, HTMLAttributes: { class: 'collab-doc-pro__table' } }),
      DocTableRow,
      DocTableHeader,
      DocTableCell,
      DocTableHandle,
      Image.configure({ inline: false, allowBase64: true, HTMLAttributes: { class: 'collab-doc-pro__image' } }),
      TaskList,
      TaskItem.configure({ nested: true }),
      ...(ydoc ? [Collaboration.configure({ document: ydoc, field: 'docx-body' })] : []),
      ...(ydoc && handle ? [CollaborationCursor.configure({
        provider: handle.provider,
        user: { name: props.displayName, color: '#58a6ff' },
      })] : []),
    ],
    content: paragraphsToContent(paragraphs),
    onUpdate: ({ editor: ed }) => onEditorUpdate(ed),
    // v0.7.106 — auto-wrap typing in an `ins` mark while trackChangesOn is true.
    // v0.7.106.1 — pure-delete detection is handled separately in the
    // view.dispatch wrapper installed by `installTrackChangesDispatchInterceptor`
    // (called right after this editor instance is created below), because
    // onTransaction fires AFTER state.doc is updated and the OLD content is
    // already gone. For inserts, state.doc is post-tr which is exactly what
    // we need to compute the wrap range, so onTransaction works fine here.
    onTransaction: ({ transaction: tr, editor: ed }) => {
      if (!trackChangesOn.value || !tr.docChanged) return
      if (tr.getMeta('trackIgnore')) return
      // Pure insertions only: from === to + non-empty slice.
      const wrapRanges: Array<{ from: number; to: number }> = []
      for (let i = 0; i < tr.steps.length; i++) {
        const step = tr.steps[i]
        const j = step.toJSON?.() as {
          stepType?: string; from?: number; to?: number
          slice?: { content?: Array<{ text?: string; size?: number }> }
        } | undefined
        if (!j || j.stepType !== 'replace') continue
        if (typeof j.from !== 'number' || typeof j.to !== 'number') continue
        const content = j.slice?.content ?? []
        if (!content.length) continue
        if (j.from !== j.to) continue
        let sz = 0
        for (const node of content) {
          if (typeof node.size === 'number') sz += node.size
          else if (typeof node.text === 'string') sz += node.text.length
          else sz += 1
        }
        if (sz <= 0) continue
        // tr.mapping.maps[i].map(j.from) = NEW position of OLD j.from.
        // The insert occupies [mapped-from - sz, mapped-from) in the NEW doc.
        const mappedFrom = tr.mapping.maps[i] ? tr.mapping.maps[i].map(j.from) : j.from
        wrapRanges.push({ from: mappedFrom - sz, to: mappedFrom })
      }
      if (!wrapRanges.length) return
      const insType = ed.schema.marks.ins
      if (!insType) return
      const max = tr.doc.content.size
      const safe = wrapRanges.filter((r) => r.from >= 0 && r.to <= max && r.from < r.to)
      if (!safe.length) return
      const author = props.displayName || 'admin'
      const date = new Date().toISOString()
      const wrapTr = ed.state.tr
      for (const r of safe) {
        wrapTr.addMark(r.from, r.to, insType.create({ author, date, id: String(++trackChangeSeq) }))
      }
      wrapTr.setMeta('trackIgnore', true)
      ed.view.dispatch(wrapTr)
    },
    // v0.7.106 — auto-wrap typing in an `ins` mark while trackChangesOn is true.
    // Transactions carrying the `trackIgnore` meta (programmatic edits from
    // collectRevisions / accept / reject helpers) skip the wrap so they don't
    // recursively re-trigger.
    editorProps: {
      attributes: { class: 'collab-doc-pro__surface' },
      handleKeyDown(_view, event) {
        // v0.7.74 — Word Alt+Shift+Up / Alt+Shift+Down moves blocks past the neighbor.
        if (event.altKey && event.shiftKey && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) {
          if (!editor.value) return false
          event.preventDefault()
          return moveBlocks(editor.value, event.key === 'ArrowUp' ? -1 : 1)
        }
        // v0.7.75 — Word Shift+F3: cycle case on the current selection
        if (event.shiftKey && (event.key === 'F3' || (event.code === 'F3'))) {
          if (!editor.value) // v0.7.78 — Word Ctrl+Space: clear formatting
        if ((event.ctrlKey || event.metaKey) && event.code === 'Space') {
          if (!editor.value) return false
          event.preventDefault()
          onClearFormat()
          return true
        }
        return false
          event.preventDefault()
          onCycleCase()
          return true
        }
        // v0.7.78 — Word Ctrl+Space: clear formatting
        if ((event.ctrlKey || event.metaKey) && event.code === 'Space') {
          if (!editor.value) return false
          event.preventDefault()
          onClearFormat()
          return true
        }
        return false
      },
    },
    onSelectionUpdate: ({ editor: ed }) => {
      // ed is the tiptap core Editor which exposes the same .state API
      // as the vue-3 wrapper we hold in editor.value.
      void ed
      refreshAiSelection()
      // v0.7.49 — comment anchor follows the selection (paragraph range)
      const sel = ed.state.selection
      if (sel && sel.from !== sel.to) {
        const fromIdx = findDocxIndexAt(ed, sel.from)
        const toIdx = findDocxIndexAt(ed, sel.to)
        commentAnchor.value =
          fromIdx != null && toIdx != null
            ? { type: 'doc', ref: JSON.stringify({ from: fromIdx, to: toIdx }) }
            : null
      } else {
        commentAnchor.value = null
      }
      // v0.7.38 — broadcast the local selection range so other
      // collaborators can render a highlight rectangle over the
      // selected text. publishSelection is idempotent on identical
      // {from,to}; the awareness layer merges via y-protocols.
      try {
        const sel = ed.state.selection
        if (sel && handle) {
          handle.publishSelection(sel.from, sel.to)
        }
      } catch (e) {
        // never block the editor on a wire failure
        // eslint-disable-next-line no-console
        console.warn('[CollabDocProEditor] publishSelection failed', e)
      }
    },
  })
  // v0.7.106.1 — view.dispatch interceptor for pure-delete auto-marking.
  // Captures OLD doc text BEFORE applying the transaction so we can re-insert
  // the deleted text with a `del` mark (Word's track-changes behavior: the
  // text stays visible but is marked as deleted; OOXML emits <w:del><w:delText>).
  if (typeof window !== 'undefined') {
    const currentEditor = editor.value
    if (!currentEditor) return
    const view = currentEditor.view
    const origDispatch = view.dispatch.bind(view)
    ;(view as any).dispatch = (tr: any) => {
      if (!trackChangesOn.value || !tr.docChanged || tr.getMeta('trackIgnore')) {
        return origDispatch(tr)
      }
      // Capture pure-delete ranges from the OLD doc.
      const deletes: Array<{ insertAt: number; text: string }> = []
      const oldDoc = currentEditor.state.doc
      for (let i = 0; i < tr.steps.length; i++) {
        const step = tr.steps[i]
        const j = step.toJSON?.() as {
          stepType?: string; from?: number; to?: number
          slice?: { content?: unknown[] }
        } | undefined
        if (!j || j.stepType !== 'replace') continue
        if (typeof j.from !== 'number' || typeof j.to !== 'number') continue
        const content = j.slice?.content ?? []
        if (content.length) continue
        if (j.from >= j.to) continue
        // Pure delete: capture text from OLD doc
        const text = oldDoc.textBetween(j.from, j.to, '\n', '\n')
        if (!text) continue
        // tr.mapping.maps[i].map(j.to) = NEW position of OLD j.to = insertion point
        const mappedTo = tr.mapping.maps[i] ? tr.mapping.maps[i].map(j.to) : j.to
        deletes.push({ insertAt: mappedTo, text })
      }
      // Apply the original transaction
      origDispatch(tr)
      // Dispatch follow-up wrap transaction(s) for deletes
      const delType = currentEditor.schema.marks.del
      if (deletes.length && delType) {
        const author = props.displayName || 'admin'
        const date = new Date().toISOString()
        const wrapTr = currentEditor.state.tr
        // Sort descending so earlier insertions don't shift later insertion sites.
        for (const d of deletes.slice().sort((a, b) => b.insertAt - a.insertAt)) {
          const mark = delType.create({ author, date, id: String(++trackChangeSeq) })
          wrapTr.insert(d.insertAt, currentEditor.schema.text(d.text, [mark]))
        }
        wrapTr.setMeta('trackIgnore', true)
        ;(view as any).dispatch.call(view, wrapTr)
      }
    }
  }

    // v0.7.103 — expose editor on window for E2E tests to inject tracked revisions.
  if (typeof window !== 'undefined') {
    ;(window as any).__wkDocEditor = editor.value
    // v0.7.106 — expose the track-changes toggle + a programmatic wrapper so
    // Playwright can drive recording mode without clicking the toolbar.
    ;(window as any).__wkDocTrackChanges = {
      get on() {
        return trackChangesOn.value
      },
      set on(v: boolean) {
        trackChangesOn.value = !!v
      },
      toggle() {
        onToggleTrackChanges()
      },
    }
  }
}

const paragraphsToContent = (paragraphs: DocxAdapterParagraph[]) => {
  const nodes: any[] = []
  for (const p of paragraphs) {
    if (p.hidden) continue
    const text = p.text || ''
    // v0.7.50 — restore the comment mark highlight for paragraphs that carry
    // comment ids in the original .docx (run-level + cross-paragraph ranges).
    const marks = p.commentIds?.length
      ? [{ type: 'comment', attrs: { ids: p.commentIds.join(' ') } }]
      : undefined
    const textNode = (t: string) => (t ? [{ type: 'text', text: t, ...(marks ? { marks } : {}) }] : [])
    if (p.kind === 'heading' && p.level) {
      nodes.push({
        type: 'heading',
        attrs: { level: p.level, 'data-docx-index': p.index },
        content: textNode(text),
      })
    } else if (p.kind === 'listItem') {
      nodes.push({
        type: 'bulletList',
        content: [{
          type: 'listItem',
          attrs: { 'data-docx-index': p.index },
          content: [{ type: 'paragraph', content: textNode(text) }],
        }],
      })
    } else {
      nodes.push({
        type: 'paragraph',
        attrs: { 'data-docx-index': p.index },
        content: textNode(text),
      })
    }
  }
  if (nodes.length === 0) {
    nodes.push({ type: 'paragraph', attrs: { 'data-docx-index': 0 }, content: [] })
  }
  return { type: 'doc', content: nodes }
}

const onEditorUpdate = (ed: import('@tiptap/core').Editor) => {
  // v0.7.71 — bump outline tick so the outline pane recomputes on every keystroke.
  outlineTick.value++
  if (!doc) {
    // First-time empty edit: we don't yet have a parsed.docx, so the save
    // path will fall back to building a blank docx from TipTap text.
    saveLabel.value = '有未保存的修改'
    scheduleSave()
    return
  }
  // Walk TipTap JSON, find each paragraph with data-docx-index, and patch
  // the matching block. Suppress the Y observer during this transaction
  // to prevent an update echo.
  const json = ed.getJSON() as { content?: any[] }
  const seen = new Set<number>()
  for (const node of json.content || []) {
    const idx = parseDocxIndex(node)
    if (idx == null || seen.has(idx)) continue
    seen.add(idx)
    // v0.7.49 — formula block: replace the whole paragraph XML with genXml
    if (node.type === 'docProtected' && typeof node.attrs?.genXml === 'string') {
      const genXml = node.attrs.genXml as string
      const preview = String(node.attrs.previewText ?? '')
      if (doc.patched.get(idx) === genXml && doc.paragraphs[idx]?.text === preview) continue
      suppressObserver = true
      try {
        doc.patched.set(idx, genXml)
        if (doc.paragraphs[idx]) doc.paragraphs[idx].text = preview
      } finally {
        suppressObserver = false
      }
      continue
    }
    const text = extractText(node)
    if (!doc.paragraphs[idx] || doc.paragraphs[idx].text === text) continue
    suppressObserver = true
    try {
      const patched = patchParagraphText(doc, idx, text)
      patchedMap.set(idx, patched.xml)
      doc.paragraphs[idx].text = text
    } finally {
      suppressObserver = false
    }
  }
  saveLabel.value = '有未保存的修改'
  scheduleSave()
}

const parseDocxIndex = (node: any): number | null => {
  if (!node || typeof node !== 'object') return null
  const attrs = node.attrs || {}
  const raw = attrs['data-docx-index'] ?? attrs.docxIndex
  if (typeof raw === 'number') return raw
  if (typeof raw === 'string' && raw !== '') return Number(raw)
  if (node.content && Array.isArray(node.content)) {
    for (const child of node.content) {
      const v = parseDocxIndex(child)
      if (v != null) return v
    }
  }
  return null
}

const extractText = (node: any): string => {
  if (!node) return ''
  if (node.type === 'text' && typeof node.text === 'string') return node.text
  if (Array.isArray(node.content)) {
    return node.content.map((c: any) => extractText(c)).join('')
  }
  return ''
}

const scheduleSave = () => {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(() => flushSave(false), 1500)
}

const flushSave = async (immediate: boolean) => {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
  if (!doc) {
    // Build a minimal docx from current editor content if we never loaded
    // one from the backend. This handles the "fresh doc, no upload yet" path.
    if (!editor.value) return
    try {
      doc = await buildBlankDocxFromEditor(editor.value)
    } catch (e: any) {
      saveError.value = e?.message || String(e)
      return
    }
  }
  const curDoc = doc
  saveLabel.value = '保存中...'
  try {
    const patched = Array.from(patchedMap.entries()).map(([docxIndex, xml]) => ({
      docxIndex, xml, text: curDoc.paragraphs[docxIndex]?.text || '',
    }))
    // v0.7.51 — full SavePlan: docxIndex-anchored blocks (paragraph insert/
    // delete + docProtected formula genXml) round-trip through saveDocx.
    const bytes = editor.value
      ? await saveDocxBytesWithImages(
          curDoc,
          editor.value.getJSON() as any,
          pmDocToSavePlan(editor.value.getJSON() as any, curDoc).blocks,
          {
            comments: commentsToCommentInfo(cachedComments),
            ...(pendingHeader.value ? { header: pendingHeader.value } : {}),
            ...(pendingFooter.value ? { footer: pendingFooter.value } : {}),
            ...(pendingProtection.value !== undefined
              ? { protection: pendingProtection.value }
              : {}),
            ...(pendingWriteProtection.value !== undefined
              ? { writeProtection: pendingWriteProtection.value }
              : {}),
          },
        )
      : await saveDocxBytes(curDoc, patched)
    patchedMap.clear()
    await uploadCollabDocBytes(props.docId, bytes, `${props.title || 'collab-doc'}.docx`)
    saveLabel.value = immediate ? '已保存' : '自动保存'
    saveError.value = null
    setTimeout(() => {
      if (saveLabel.value === '已保存' || saveLabel.value === '自动保存') saveLabel.value = '已保存'
    }, 1500)
  } catch (e: any) {
    saveError.value = e?.message || String(e)
    saveLabel.value = '保存失败'
  }
}

const buildBlankDocxFromEditor = async (ed: Editor): Promise<DocxAdapterDocument> => {
  // Synthesize a minimal docx from the current TipTap content so the
  // first save can produce real .docx bytes. We pull each top-level
  // paragraph and seed buildBlankDocxDoc with the first line; subsequent
  // edits continue through patchParagraphText.
  const json = ed.getJSON() as { content?: any[] }
  const paragraphs: Array<{ text: string; kind: 'paragraph' | 'heading' | 'listItem'; level?: number }> = []
  for (const node of json.content || []) {
    const text = extractText(node)
    const kind = node.type === 'heading' ? 'heading' as const : 'paragraph' as const
    const level = node.type === 'heading' && node.attrs?.level ? Number(node.attrs.level) : undefined
    paragraphs.push({ text, kind, level })
    if (paragraphs.length >= 1) break // first paragraph becomes the docx seed
  }
  return buildBlankDocxDoc(paragraphs)
}

const teardown = () => {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
  if (editor.value) {
    editor.value.destroy()
    editor.value = undefined
  }
  if (handle) {
    handle.destroy()
    handle = null
  }
  doc = null
  ydoc = null
}

setup()
watch(() => props.docId, () => { teardown(); setup() })
onBeforeUnmount(teardown)
</script>

<style scoped>

/* v0.7.74 — DOC Ribbon group styling (matches GenOffice ribbon-vertical-divider + group label). */
/* v0.7.78 — DOC 大按钮（图标 + 文字下方，仿 GenOffice BIG=28） */
/* v0.7.79 — DOC 大按钮：GenOffice .rb-big 标准（28px 图标 + 12px 文字 + 紧凑 padding） */
.collab-doc-pro__btn--icon {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  padding: 4px 7px 6px;
  min-width: 36px;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 4px;
  color: var(--app-text, #1f232b);
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
}
.collab-doc-pro__btn--icon:hover:not(:disabled) {
  background: var(--rb-hover, rgba(24, 90, 189, 0.08));
  border-color: transparent;
}
.collab-doc-pro__btn--icon:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.collab-doc-pro__btn--icon svg {
  display: block;
  width: 28px;
  height: 28px;
}
.collab-doc-pro__btn--icon span {
  white-space: nowrap;
  line-height: 1.15;
}

.collab-doc-pro__ribbon-group {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 4px 10px 6px;
  gap: 4px;
  min-width: 64px;
  border-left: 1px solid var(--app-border, #2c313b);
}
.collab-doc-pro__ribbon-group:first-child {
  border-left: 0;
}
.collab-doc-pro__ribbon-group-label {
  font-size: 10px;
  color: #7c8696;
  text-transform: none;
  letter-spacing: 0.02em;
  user-select: none;
}
.collab-doc-pro__ribbon-group .collab-doc-pro__btn {
  font-size: 12px;
  padding: 4px 10px;
  background: transparent;
  border: 1px solid transparent;
  color: var(--app-text, #dce4ed);
}
.collab-doc-pro__ribbon-group .collab-doc-pro__btn:hover {
  background: rgba(90, 168, 255, 0.08);
  border-color: rgba(90, 168, 255, 0.3);
}
.collab-doc-pro__ribbon-group .collab-doc-pro__btn--ai {
  color: #5aa8ff;
  font-weight: 600;
}
.collab-doc-pro__ribbon-group .collab-doc-pro__btn--ai:hover {
  background: rgba(90, 168, 255, 0.15);
}


.collab-doc-pro__revisions-item-head { display: flex; align-items: center; gap: 6px; padding: 4px 0; flex-wrap: wrap; }
/* v0.7.106 — per-revision timestamp rendered between snippet and actions. */
.collab-doc-pro__revisions-date { color: var(--td-text-color-secondary, #888); font-size: 11px; white-space: nowrap; }
.collab-doc-pro__btn--active { background: #fef3c7; border-color: #f59e0b; color: #92400e; }
.collab-doc-pro__btn--active:hover { background: #fde68a; }
.collab-doc-pro__revisions-item-actions { display: inline-flex; gap: 4px; margin-left: auto; }
.collab-doc-pro__revisions-mini { background: transparent; border: 1px solid var(--td-component-stroke); border-radius: 4px; padding: 2px 8px; font-size: 11px; cursor: pointer; }
.collab-doc-pro__revisions-mini:hover { background: #f0f3f7; }
.collab-doc-pro__revisions-mini.accept { color: var(--td-success-color, #2da44e); border-color: #2da44e; }
.collab-doc-pro__revisions-mini.reject { color: var(--td-error-color, #d54941); border-color: #d54941; }
.collab-doc-pro__revisions-kind--ins, .collab-doc-pro__revisions-kind--blockIns { color: #2da44e; background: rgba(45,164,78,0.10); padding: 1px 6px; border-radius: 3px; font-size: 11px; }
.collab-doc-pro__revisions-kind--del, .collab-doc-pro__revisions-kind--blockDel { color: #d54941; background: rgba(213,73,65,0.10); padding: 1px 6px; border-radius: 3px; font-size: 11px; text-decoration: line-through; }
.collab-doc-pro__revisions-snippet { font-family: 'JetBrains Mono', ui-monospace, monospace; font-size: 11px; color: var(--td-text-color-secondary); max-width: 320px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.collab-doc-pro__math-bg {
  position: fixed; inset: 0;
  background: rgba(0,0,0,0.4);
  display: flex; align-items: center; justify-content: center;
  z-index: 100;
}
.collab-doc-pro__math {
  background: var(--td-bg-color-container);
  padding: 20px;
  border-radius: 8px;
  min-width: 420px;
  max-width: 80vw;
  display: flex; flex-direction: column; gap: 12px;
}
.collab-doc-pro__math h3 { margin: 0; font-size: 16px; }
.collab-doc-pro__math-input {
  padding: 8px 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 4px;
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 13px;
  resize: vertical;
}
.collab-doc-pro__math-preview {
  padding: 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 4px;
  background: var(--td-bg-color-secondarycontainer);
  min-height: 60px;
  font-family: serif;
  text-align: center;
  overflow-x: auto;
}
.collab-doc-pro__math-error {
  color: var(--td-error-color-7);
  font-size: 12px;
  margin: 0;
}
.collab-doc-pro__math-actions {
  display: flex; gap: 8px; justify-content: flex-end;
}
.collab-doc-pro__math-actions button {
  padding: 6px 14px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 4px;
  background: var(--td-bg-color-container);
  cursor: pointer; font-size: 12px;
}
.collab-doc-pro__math-actions button:last-child {
  background: var(--td-brand-color-7); color: white; border-color: var(--td-brand-color-7);
}
.collab-doc-pro__math-block {
  display: block;
  padding: 8px 12px;
  margin: 8px 0;
  background: var(--td-bg-color-secondarycontainer);
  border-left: 3px solid var(--td-brand-color-7);
  text-align: center;
  font-family: serif;
}
.collab-doc-pro { display: flex; flex-direction: column; height: 100%; }
.collab-doc-pro__toolbar {
  display: flex; align-items: center; gap: 10px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  flex-wrap: wrap;
}
.collab-doc-pro__title { font-weight: 600; font-size: 14px; }
.collab-doc-pro__kind { font-size: 11px; padding: 2px 8px; border-radius: 999px; background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-doc-pro__connection { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-secondary); }
.collab-doc-pro__connection.connected { background: var(--td-success-color-1); color: var(--td-success-color-7); }
.collab-doc-pro__savetag { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-secondary); }
.collab-doc-pro__savetag.dirty { background: var(--td-warning-color-1); color: var(--td-warning-color-7); }
.collab-doc-pro__savetag.saving { background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-doc-pro__btn { padding: 4px 12px; font-size: 12px; border: 1px solid var(--td-component-stroke); border-radius: 4px; background: var(--td-bg-color-container); cursor: pointer; }
.collab-doc-pro__btn:disabled { opacity: 0.5; cursor: not-allowed; }
.collab-doc-pro__peers { display: flex; gap: 4px; margin-left: auto; }
.collab-doc-pro__peer {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 999px;
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  border: 2px solid transparent;
  transition: border-color 120ms ease, box-shadow 120ms ease;
}
.collab-doc-pro__peer--selecting {
  border-color: rgba(255, 255, 255, 0.85);
  box-shadow: 0 0 0 2px rgba(0, 0, 0, 0.18);
}
.collab-doc-pro__peer { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; color: white; font-size: 11px; font-weight: 600; }
/* v0.7.77 — without this, the formatbar + surface-wrap siblings sit at
   their content height (≈145px) because .collab-doc-pro__main has no
   flex grow factor, leaving the ProseMirror .collab-doc-pro__surface
   with no remaining height inside the 720px viewport.  Add flex:1 +
   display:flex-column + min-height:0 so the wrap child can claim
   the rest of the viewport. */
.collab-doc-pro__main {
  flex: 1 1 0%;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.collab-doc-pro__formatbar {
  flex: 0 0 auto;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--app-border);
  background: var(--app-surface-raised);
}
/* v0.7.79 — DOC 编辑区：Word 风格白纸感（默认白底，dark mode 下保留深色） */
.collab-doc-pro__surface-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  background: #eef1f5; /* Word canvas gray */
}
:root[theme-mode='dark'] .collab-doc-pro__surface-wrap {
  background: #1d2024;
}
.collab-doc-pro__surface {
  flex: 1;
  overflow: auto;
  padding: 48px 64px;
  max-width: 880px;
  margin: 24px auto;
  width: 100%;
  background: #ffffff; /* Word paper white */
  color: #1a1a1a;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08), 0 8px 24px rgba(0, 0, 0, 0.06);
  border-radius: 2px;
  min-height: calc(100% - 48px);
}
:root[theme-mode='dark'] .collab-doc-pro__surface {
  background: #2a2d33;
  color: #e9ecef;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3), 0 8px 24px rgba(0, 0, 0, 0.4);
}
/* v0.7.77 — keep the comments panel pinned to its content height so
   the editor surface can claim the remaining viewport without being
   squeezed by a tall comments tree. */
.collab-doc-pro > .collab-comments { flex: 0 0 auto; }
.collab-doc-pro__loading, .collab-doc-pro__error { padding: 24px; }
.collab-doc-pro__error { color: var(--td-error-color-7); }
.collab-doc-pro__fmt { padding: 5px 9px; border: 1px solid transparent; border-radius: 5px; background: transparent; color: var(--app-text-muted); cursor: pointer; }
.collab-doc-pro__fmt:hover { background: var(--app-surface-bg); color: var(--app-text); }
.collab-doc-pro__fmt.active { background: color-mix(in srgb, var(--td-brand-color) 16%, var(--app-surface-bg)); color: var(--td-brand-color); border-color: color-mix(in srgb, var(--td-brand-color) 40%, var(--app-border)); }

.collab-doc-pro__outline {
  border-top: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  max-height: 280px;
  display: flex;
  flex-direction: column;
}
.collab-doc-pro__outline-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 600;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-secondarycontainer);
}
.collab-doc-pro__outline-count {
  font-size: 11px;
  font-weight: 400;
  color: var(--td-text-color-secondary);
}
.collab-doc-pro__outline-empty {
  padding: 16px 12px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  text-align: center;
}
.collab-doc-pro__outline-list {
  flex: 1 1 auto;
  overflow: auto;
  padding: 4px 0;
}
.collab-doc-pro__outline-item {
  display: block;
  width: 100%;
  border: none;
  background: transparent;
  text-align: left;
  padding: 4px 12px;
  font-size: 12px;
  cursor: pointer;
  color: var(--td-text-color-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  border-radius: 0;
}
.collab-doc-pro__outline-item:hover {
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-brand-color-7);
}
.collab-doc-pro__outline-item--l1 {
  font-weight: 600;
  padding-left: 12px;
  font-size: 13px;
}
.collab-doc-pro__outline-item--l2 {
  padding-left: 24px;
  color: var(--td-text-color-secondary);
}
.collab-doc-pro__outline-item--l3 {
  padding-left: 36px;
  color: var(--td-text-color-secondary);
}
.collab-doc-pro__outline-item--l4 {
  padding-left: 48px;
  color: var(--td-text-color-disabled);
  font-size: 11px;
}
/* v0.7.72 — DOC page-setup / multi-section preview modal */
.collab-doc-pro__sections { width: min(720px, 92vw); max-height: 80vh; display: flex; flex-direction: column; }
/* v0.7.73 — DOC find / replace panel */
.collab-doc-pro__find { width: min(560px, 92vw); display: flex; flex-direction: column; gap: 10px; }
.collab-doc-pro__find-field { display: flex; gap: 10px; align-items: center; }
.collab-doc-pro__find-label { flex: 0 0 80px; color: var(--td-text-color-secondary, #888); font-size: 13px; }
.collab-doc-pro__find-input { flex: 1 1 auto; padding: 6px 10px; border: 1px solid var(--td-component-stroke, #d0d7de); border-radius: 4px; font-size: 13px; outline: none; }
.collab-doc-pro__find-input:focus { border-color: var(--td-brand-color-7, #0052d9); }
.collab-doc-pro__find-opts { display: flex; gap: 16px; padding-left: 90px; }
.collab-doc-pro__find-opt { font-size: 13px; color: var(--td-text-color-secondary, #555); display: flex; gap: 4px; align-items: center; cursor: pointer; }
.collab-doc-pro__find-status { padding: 6px 90px 0 90px; font-size: 12px; color: var(--td-text-color-secondary, #888); }
.collab-doc-pro__find-actions { display: flex; gap: 8px; flex-wrap: wrap; padding-top: 4px; }
.collab-doc-pro__find-actions button { padding: 6px 12px; border: 1px solid var(--td-component-stroke, #d0d7de); border-radius: 4px; background: var(--td-bg-color-container, #fff); cursor: pointer; font-size: 13px; }
.collab-doc-pro__find-actions button:hover:not(:disabled) { border-color: var(--td-brand-color-7, #0052d9); color: var(--td-brand-color-7, #0052d9); }
.collab-doc-pro__find-actions button:disabled { opacity: 0.5; cursor: not-allowed; }
.collab-doc-pro__sections-body { display: flex; gap: 16px; min-height: 360px; max-height: 60vh; }
.collab-doc-pro__sections-list { flex: 0 0 220px; display: flex; flex-direction: column; gap: 4px; overflow-y: auto; border: 1px solid var(--td-component-stroke, #e7e7e7); border-radius: 6px; padding: 6px; }
.collab-doc-pro__sections-item { text-align: left; background: transparent; border: 1px solid transparent; border-radius: 4px; padding: 6px 10px; cursor: pointer; font-size: 13px; line-height: 1.4; }
.collab-doc-pro__sections-item:hover { background: var(--td-bg-color-container-hover, #f2f3f5); }
.collab-doc-pro__sections-item.active { background: var(--td-brand-color-1, #e6f0ff); border-color: var(--td-brand-color-7, #0052d9); color: var(--td-brand-color-7, #0052d9); font-weight: 500; }
.collab-doc-pro__sections-empty { color: var(--td-text-color-secondary, #888); padding: 16px; text-align: center; font-size: 12px; }
.collab-doc-pro__sections-detail { flex: 1 1 auto; display: flex; flex-direction: column; gap: 8px; padding: 12px; border: 1px solid var(--td-component-stroke, #e7e7e7); border-radius: 6px; overflow-y: auto; }
.collab-doc-pro__sections-row { display: flex; gap: 12px; font-size: 13px; }
.collab-doc-pro__sections-label { flex: 0 0 80px; color: var(--td-text-color-secondary, #888); }
.collab-doc-pro__sections-value { flex: 1 1 auto; color: var(--td-text-color-primary, #222); }
.collab-doc-pro__math input,
.collab-doc-pro__math textarea,
.collab-doc-pro__find-input,
.collab-doc-pro__protect input,
.collab-doc-pro__compare-upload input { background: var(--app-control-bg); color: var(--app-text); }
.collab-doc-pro__math-actions button,
.collab-doc-pro__find-actions button { color: var(--app-text); }
</style>
.collab-doc-pro__table { border-collapse: collapse; margin: 12px 0; width: 100%; table-layout: fixed; }
.collab-doc-pro__table th, .collab-doc-pro__surface :deep(table th),
.collab-doc-pro__surface :deep(table td) { border: 1px solid var(--td-component-stroke, #e7e7e7); padding: 6px 10px; vertical-align: top; min-width: 60px; }
.collab-doc-pro__surface :deep(table th) { background: var(--td-bg-color-container, #f7f7f7); font-weight: 600; }
.collab-doc-pro__surface :deep(.selectedCell) { background: rgba(88, 166, 255, 0.12); }
.doc-table-handle {
  position: absolute;
  z-index: 20;
  width: 18px;
  height: 18px;
  padding: 0;
  border: 1px solid var(--td-component-stroke, #d0d7de);
  border-radius: 4px;
  background: var(--td-bg-color-container, #fff);
  color: var(--td-text-color-secondary, #57606a);
  cursor: grab;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.12);
}
.doc-table-handle:hover { color: var(--td-brand-color, #0052d9); border-color: var(--td-brand-color, #0052d9); }
.doc-table-handle:active { cursor: grabbing; }
.collab-doc-pro__image, .collab-doc-pro__surface :deep(img) { max-width: 100%; height: auto; border-radius: 4px; margin: 8px 0; }
.collab-doc-pro__surface :deep(ul[data-type="taskList"]) { list-style: none; padding-left: 0; }
.collab-doc-pro__surface :deep(ul[data-type="taskList"] li) { display: flex; gap: 6px; align-items: flex-start; }
.collab-doc-pro__surface :deep(ul[data-type="taskList"] li > label) { flex: 0 0 auto; margin-top: 4px; }
.collab-doc-pro__surface :deep(ul[data-type="taskList"] li > div) { flex: 1 1 auto; }
.collab-doc-pro__surface :deep(mark) { background: color-mix(in srgb, var(--td-warning-color) 35%, transparent); padding: 0 2px; border-radius: 2px; }
