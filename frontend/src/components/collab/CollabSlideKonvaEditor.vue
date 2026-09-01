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
  <div class="collab-slide-konva">
    <div class="collab-slide-konva__toolbar">
      <span class="collab-slide-konva__title">{{ title }}</span>
      <span class="collab-slide-konva__kind">{{ kindLabel }}</span>
      <span class="collab-slide-konva__connection" :class="{ connected: connected && !saveError }">
        {{ connectionLabel }}
      </span>
      <span class="collab-slide-konva__savetag" :class="savetagClass">
        {{ saveLabel }}
      </span>
      <button @click="addShape('text')" type="button" data-testid="slide-add-text">+ 文本框</button>
      <button @click="addShape('rect')" type="button" data-testid="slide-add-rect">+ 矩形</button>
      <button @click="addShape('roundRect')" type="button" title="圆角矩形">+ 圆角矩形</button>
      <button @click="addShape('ellipse')" type="button" data-testid="slide-add-ellipse">+ 椭圆</button>
      <button @click="addShape('line')" type="button" data-testid="slide-add-line">+ 直线</button>
      <button @click="addShape('arrow')" type="button" title="右箭头">→ 箭头</button>
      <button @click="addShape('triangle')" type="button" title="三角形">△ 三角</button>
      <button @click="addShape('star')" type="button" title="五角星">★ 星</button>
      <button @click="addShape('hexagon')" type="button" title="六边形">⬡ 六边</button>
      <button @click="addShape('callout')" type="button" title="对话气泡">💬 对话</button>
      <button @click="promptAddTable" type="button" title="插入表格">⊞ 表格</button>
      <button @click="addSlide" type="button">+ 新建幻灯片</button>
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
      <button @click="triggerUpload" type="button" :disabled="uploading">
        {{ uploading ? '上传中…' : '上传 .pptx' }}
      </button>
      <input ref="fileInput" type="file" accept=".pptx" style="display:none" @change="onUploadFile" />
      <button @click="onDownload" type="button" :disabled="downloading">
        {{ downloading ? '下载中…' : '下载 .pptx' }}
      </button>
      <button
        type="button"
        class="collab-slide-konva__present-btn"
        data-testid="slide-present-btn"
        :disabled="!slides.length || loading"
        @click="onEnterPresent"
        title="全屏演示 (F5)"
      >▶ 演示</button>
      <button @click="deleteSelected" type="button" :disabled="!selectedId" data-testid="slide-delete-selected">删除</button>
      <button @click="duplicateSelected" type="button" :disabled="!selectedId" title="复制所选">⎘ 复制</button>
      <button @click="bringForward" type="button" :disabled="!selectedId" title="上移一层">↑ 上移</button>
      <button @click="sendBackward" type="button" :disabled="!selectedId" title="下移一层">↓ 下移</button>
      <button @click="bringToFront" type="button" :disabled="!selectedId" title="置于顶层">⤒ 顶层</button>
      <button @click="sendToBack" type="button" :disabled="!selectedId" title="置于底层">⤓ 底层</button>
      <button @click="rotateSelected(-90)" type="button" :disabled="!selectedId" title="逆时针旋转 90° (Alt+←)" data-testid="slide-rotate-ccw">↺ 旋转</button>
      <button @click="rotateSelected(90)" type="button" :disabled="!selectedId" title="顺时针旋转 90° (Alt+→)" data-testid="slide-rotate-cw">↻ 旋转</button>
      <span class="collab-slide-konva__divider" />
      <button @click="onUndo" type="button" :disabled="!canUndo" title="撤销 (Ctrl+Z)">↶ 撤销</button>
      <button @click="onRedo" type="button" :disabled="!canRedo" title="重做 (Ctrl+Shift+Z)">↷ 重做</button>
      <span class="collab-slide-konva__peers">
        <span
          v-for="p in peers"
          :key="p.clientId"
          class="collab-slide-konva__peer"
          :style="{ backgroundColor: p.color }"
          :title="p.displayName"
        >{{ initialOf(p.displayName) }}</span>
      </span>
    </div>
    <div v-if="loading" class="collab-slide-konva__loading">加载演示文稿中…</div>
    <div v-else class="collab-slide-konva__body">
      <p v-if="recoveryMessage" class="collab-slide-konva__recovery">{{ recoveryMessage }}</p>
      <aside class="collab-slide-konva__thumbs">
        <button
          v-for="(s, i) in slides"
          :key="i"
          class="collab-slide-konva__thumb"
          :class="{ active: i === activeIndex }"
          @click="activeIndex = i"
        >
          <span class="collab-slide-konva__thumb-num">{{ i + 1 }}</span>
          <span class="collab-slide-konva__thumb-title">{{ slideSummary(s) }}</span>
          <button class="collab-slide-konva__iconbtn" @click.stop="moveSlide(i, i - 1)" :disabled="i === 0" title="上移">↑</button>
          <button class="collab-slide-konva__iconbtn" @click.stop="moveSlide(i, i + 1)" :disabled="i === slides.length - 1" title="下移">↓</button>
          <button class="collab-slide-konva__iconbtn danger" @click.stop="deleteSlide(i)" :disabled="slides.length <= 1" title="删除">×</button>
        </button>
      </aside>
      <div class="collab-slide-konva__stage-wrap">
        <div class="collab-slide-konva__zoom-info">{{ stageWidthPx }}×{{ stageHeightPx }} px</div>
        <v-stage
          v-if="activeSlide"
          ref="stageRef"
          :config="stageConfig"
          class="collab-slide-konva__stage"
          @click="onStageClick"
          @tap="onStageClick"
        >
          <v-layer>
            <!-- background fill -->
            <v-rect
              v-if="activeSlide.background"
              :config="{ x: 0, y: 0, width: stageWidthPx, height: stageHeightPx, fill: '#' + activeSlide.background }"
            />
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
                  fill: shape.fill ? '#' + shape.fill : '#1f2937',
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
          <button type="button" :class="{ active: inspectorBold }" @click="toggleBold" title="粗体"><b>B</b></button>
          <button type="button" :class="{ active: inspectorItalic }" @click="toggleItalic" title="斜体"><i>I</i></button>
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
            <button type="button" class="slide-present-btn" @click="presentPrev" :disabled="presentIndex === 0" data-testid="slide-present-prev" title="上一页 (←)">← 上一页</button>
            <span class="slide-present-counter" data-testid="slide-present-counter">{{ presentIndex + 1 }} / {{ slides.length }}</span>
            <button type="button" class="slide-present-btn" @click="presentNext" :disabled="presentIndex >= slides.length - 1" data-testid="slide-present-next" title="下一页 (→)">下一页 →</button>
            <span class="slide-present-divider" />
            <button type="button" class="slide-present-btn slide-present-btn--exit" @click="onExitPresent" data-testid="slide-present-exit" title="退出 (ESC)">✕ 退出 (ESC)</button>
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
    <section class="collab-slide-konva__notes">
      <header class="collab-slide-konva__notes-header">
        <span>📝 演讲者备注</span>
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
    <section class="collab-slide-konva__animations">
      <header class="collab-slide-konva__animations-header">
        <span>🎬 动画 (第 {{ activeIndex + 1 }} 页)</span>
        <span class="collab-slide-konva__animations-status">{{ animations.length }} 个效果</span>
      </header>
      <!-- v0.7.64 — slide transition (inter-slide effect) -->
      <div class="collab-slide-konva__animations-toolbar">
        <label class="collab-slide-konva__animations-label">转场:
          <select v-model="transitionInput" @change="onTransitionCommit" class="collab-slide-konva__animations-select" title="幻灯片切换效果">
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
        <select v-model="newEffect" class="collab-slide-konva__animations-select" title="效果">
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
        <select v-model="newTrigger" class="collab-slide-konva__animations-select" title="触发">
          <option value="onClick">点击时</option>
          <option value="withPrevious">与上一动画同时</option>
          <option value="afterPrevious">上一动画之后</option>
        </select>
        <button
          type="button"
          class="collab-slide-konva__animations-btn"
          :disabled="selectedId == null"
          @click="addAnimation"
          title="为选中形状添加动画"
        >+ 添加动画</button>
        <button
          type="button"
          class="collab-slide-konva__animations-btn"
          :disabled="animations.length === 0"
          @click="clearAnimations"
        >清除</button>
      </div>
      <ol v-if="animations.length" class="collab-slide-konva__animations-list">
        <li v-for="(a, idx) in animations" :key="`${a.spId}-${idx}`" class="collab-slide-konva__animations-item">
          <span class="collab-slide-konva__animations-num">{{ idx + 1 }}</span>
          <span class="collab-slide-konva__animations-name">
            {{ effectLabel(a.effect) }}
            <small>{{ triggerLabel(a.trigger) }}</small>
          </span>
          <span class="collab-slide-konva__animations-target">spId {{ a.spId }}</span>
          <button type="button" class="collab-slide-konva__animations-del" @click="removeAnimation(idx)">×</button>
        </li>
      </ol>
      <p v-else class="collab-slide-konva__animations-empty">
        选中一个形状，然后点击「+ 添加动画」为其添加入场/强调/退出效果。
      </p>
    </section>
    <!-- v0.7.29 — comments side panel -->
    <CollabCommentsPanel
      :doc-id="props.docId"
      :token="props.token"
      :anchor="commentAnchor"
      anchor-label="当前幻灯片"
      placeholder="对当前幻灯片或所选形状添加评论…"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
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
  getSlideTransitionOnDeck,
  setSlideTransitionOnDeck,
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
    selectedId.value = newTable.id
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

const stageConfig = computed(() => ({
  width: stageWidthPx.value,
  height: stageHeightPx.value,
}))

const localSlidesByIndex = computed(() => slides.value)
const activeSlide = computed(() => slides.value[activeIndex.value] ?? null)
const activeShapes = computed(() => activeSlide.value?.shapes ?? [])
const selectedShape = computed(() => activeShapes.value.find((s) => s.id === selectedId.value) ?? null)

const connectionLabel = computed(() => (connected.value ? '在线' : '离线'))
const initialOf = (s: string) => (s || '?').trim().charAt(0).toUpperCase()

const slideSummary = (s: PptxShapeSlide) => {
  const t = s.shapes.find((x) => x.type === 'text' && x.text)
  if (t && t.text) return t.text.split('\n')[0].slice(0, 30)
  return `幻灯片 ${s.index + 1}`
}

// --- Konva stage / transformer wiring ---
const stageRef = ref<any>(null)
const publishCursor = (px: number, py: number) => {
  if (!handle) return
  // px / py are CSS pixels; convert back to EMU
  const xEmu = Math.round((px / PX_PER_INCH) * 914400)
  const yEmu = Math.round((py / PX_PER_INCH) * 914400)
  handle.provider.awareness.setLocalStateField('cursor', { slide: activeIndex.value, x: xEmu, y: yEmu })
}
/** Publish selection: which shape (if any) the user has selected on the
 *  current slide. Other collaborators see a colored dashed outline around
 *  the same shape (rendered via `remoteSelections`). */
const publishSelection = (shapeId: string | null) => {
  if (!handle) return
  handle.provider.awareness.setLocalStateField('selection', {
    slide: activeIndex.value,
    shapeId: shapeId ?? '',
  })
}
const transformerRef = ref<any>(null)

// --- v0.7.29 — comments anchor (current slide + selected shape if any) ---
const commentAnchor = ref<{ type: 'doc' | 'slide' | 'sheet'; ref: string } | null>(null)
watch([activeIndex, selectedId], ([s, id]) => {
  commentAnchor.value = {
    type: 'slide',
    ref: JSON.stringify({ slide: s, shapeId: id ?? '' }),
  }
  publishSelection(id)
}, { immediate: true })

const onStageClick = (e: any) => {
  const stage = stageRef.value?.getStage?.()
  // Click on empty stage clears selection.
  const target = e?.target
  if (!target || target === stage) {
    selectedId.value = null
    updateTransformer()
  }
  if (stage) {
    const pos = stage.getPointerPosition?.()
    if (pos) publishCursor(pos.x, pos.y)
  }
}

const onShapeClick = (id: string, _e: any) => {
  selectedId.value = id
  updateTransformer()
}

const updateTransformer = async () => {
  await nextTick()
  const stage = stageRef.value?.getStage?.()
  const tr = transformerRef.value?.getNode?.()
  if (!stage || !tr) return
  if (!selectedId.value) {
    tr.nodes([])
    tr.getLayer()?.batchDraw?.()
    return
  }
  const node = stage.findOne(`#${selectedId.value}`)
  if (node) {
    tr.nodes([node])
    tr.getLayer()?.batchDraw?.()
  }
}

const onShapeDragEnd = (id: string, e: any) => {
  const node = e?.target
  if (!node) return
  const newX = Math.round((node.x() / PX_PER_INCH) * 914400)
  const newY = Math.round((node.y() / PX_PER_INCH) * 914400)
  updateShape(id, { x: newX, y: newY })
}

const onShapeTransformEnd = (id: string, e: any) => {
  const node = e?.target
  if (!node) return
  const scaleX = node.scaleX()
  const scaleY = node.scaleY()
  // Reset scale and bake into width/height.
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
  selectedId.value = id
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
})
watch(() => deck.value?.opened, () => {
  refreshAnimations()
  loadTransitionForActive()
}, { immediate: false })

const addAnimation = () => {
  if (!deck.value || selectedId.value == null) return
  const spId = Number(selectedId.value)
  if (!Number.isFinite(spId)) {
    MessagePlugin.warning('无法为所选形状添加动画:缺少 spId')
    return
  }
  const next: SlideAnimationRecord[] = [
    ...animations.value,
    { spId, effect: newEffect.value, trigger: newTrigger.value, durationMs: 1000, delayMs: 0 },
  ]
  if (setSlideAnimationsOnDeck(deck.value, activeIndex.value, next)) {
    animations.value = next
    saveLabel.value = '动画已暂存（保存 .pptx 时落盘）'
  }
}

const removeAnimation = (idx: number) => {
  if (!deck.value) return
  const next = animations.value.filter((_, i) => i !== idx)
  if (setSlideAnimationsOnDeck(deck.value, activeIndex.value, next)) {
    animations.value = next
  }
}

const clearAnimations = () => {
  if (!deck.value) return
  if (setSlideAnimationsOnDeck(deck.value, activeIndex.value, [])) {
    animations.value = []
  }
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
    x: Number(o.x ?? 0),
    y: Number(o.y ?? 0),
    w: Number(o.w ?? 914400),
    h: Number(o.h ?? 457200),
    text: o.text ? String(o.text) : undefined,
    fill: o.fill ? String(o.fill) : undefined,
    stroke: o.stroke ? String(o.stroke) : undefined,
    strokeWidth: Number(o.strokeWidth ?? 0) || undefined,
    fontSize: Number(o.fontSize ?? 18),
    mediaRef: o.mediaRef ? String(o.mediaRef) : undefined,
    mediaData: o.mediaData ? String(o.mediaData) : undefined,
    spIndex: Number(o.spIndex ?? -1),
    sourceType: o.sourceType ? String(o.sourceType) : undefined,
    preset: o.preset ? String(o.preset) : undefined,
    rows: o.rows ? Number(o.rows) : undefined,
    cols: o.cols ? Number(o.cols) : undefined,
    cellTexts,
  }
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
}

const seedYjs = () => {
  if (!ydeck) return
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
    selectedId.value = id
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
    selectedId.value = null
    scheduleSave()
  })
}

// --- Save / load ---
const saveTimer = ref<number | null>(null)
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
    selectedId.value = newId
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
    if (sel && sel.slide === activeIndex.value && sel.shapeId) {
      selections.push({
        clientId,
        shapeId: sel.shapeId,
        color: u.color || '#58a6ff',
        name: u.name || '匿名用户',
      })
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
      headers: { Authorization: `Bearer ${props.token}` },
    })
    if (existing.ok) {
      const buf = new Uint8Array(await existing.arrayBuffer())
      if (buf.byteLength > 100) {
        if (await onLoadBytes(buf)) return
        await initializeBlankDeck('原演示文稿无法解析，已恢复为新的空白演示文稿；首次编辑后会保存为有效 PPTX。')
        return
      }
    }
  } catch { /* fall through */ }
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
.collab-slide-konva { display: flex; flex-direction: column; height: 100%; background: var(--td-bg-color-container); }
.collab-slide-konva__toolbar { display: flex; align-items: center; gap: 8px; padding: 8px 12px; border-bottom: 1px solid var(--td-component-stroke); flex-wrap: wrap; }
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
.collab-slide-konva__stage-wrap { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 24px; overflow: auto; }
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
</style>
    return { index: Number(obj.index ?? 0), width, height, background, shapes }
