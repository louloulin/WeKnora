<!--
  CollabSheetEditor — v0.7.26 SHEET-kind collaborative editor with real
  .xlsx byte round-trip.

  Architecture:
   1. Open: GET /collaborative-docs/:id/download (latest .xlsx bytes)
            -> xlsxAdapter.openXlsx(bytes) seeds a structured sheet
            (rows[i][j] = { v: cell value }).
   2. Realtime: Y.Map<rowKey, Y.Map<colKey, Y.Text>>. Two clients editing
            different cells converge via Yjs CRDT.
   3. Save: xlsxAdapter.saveXlsxBytes(wb) -> POST .../upload
            (multipart/form-data with file field).
-->
<template>
  <div class="collab-sheet-editor">
    <div class="collab-sheet-editor__toolbar">
      <span class="collab-sheet-editor__title">{{ title }}</span>
      <span class="collab-sheet-editor__kind">{{ kindLabel }}</span>
      <span class="collab-sheet-editor__connection" :class="{ connected: connected && !saveError }">
        {{ connectionLabel }}
      </span>
      <span class="collab-sheet-editor__savetag" :class="savetagClass">
        {{ saveLabel }}
      </span>
      <button class="collab-sheet-editor__add-col" @click="addColumn" type="button">+ 列</button>
      <button class="collab-sheet-editor__add-row" @click="addRow" type="button">+ 行</button>
      <button class="collab-sheet-editor__add-col" @click="delLastColumn" type="button" :disabled="cols.length <= 1">- 列</button>
      <button class="collab-sheet-editor__add-row" @click="delLastRow" type="button" :disabled="rows.length <= 1">- 行</button>
      <button class="collab-sheet-editor__upload" :disabled="uploading" @click="triggerUpload" type="button">
        {{ uploading ? '上传中...' : '上传 .xlsx' }}
      </button>
      <input
        ref="fileInput"
        type="file"
        accept=".xlsx"
        style="display:none"
        @change="onUploadFile"
      />
      <button class="collab-sheet-editor__export" :disabled="downloading" @click="exportXlsx" type="button">
        {{ downloading ? '下载中...' : '下载 .xlsx' }}
      </button>
      <button class="collab-sheet-editor__feature" @click="openFreezeModal" type="button" title="冻结窗格">冻结</button>
      <button class="collab-sheet-editor__feature" @click="openFilterModal" type="button" title="数据筛选">筛选</button>
      <button class="collab-sheet-editor__feature" @click="openCfModal" type="button" title="条件格式">条件格式</button>
      <button class="collab-sheet-editor__feature" @click="openDvModal" type="button" title="数据验证">数据验证</button>
      <button class="collab-sheet-editor__feature" @click="openSparkModal" type="button" title="迷你图">迷你图</button>
      <button class="collab-sheet-editor__feature" @click="openPageSetupModal" type="button" title="页面布局">页面</button>
      <button class="collab-sheet-editor__feature" @click="openSheetManageModal" type="button" title="工作表管理">工作表</button>
      <button class="collab-sheet-editor__feature" @click="openNoteModal" type="button" title="单元格批注">批注</button>
      <button class="collab-sheet-editor__feature" @click="openHyperlinkModal" type="button" title="超链接">链接</button>
      <button class="collab-sheet-editor__feature" @click="openFindModal" type="button" title="查找替换" data-testid="sheet-find-btn">查找</button>
      <button class="collab-sheet-editor__feature" @click="openSortModal" type="button" title="按列排序" data-testid="sheet-sort-btn">排序</button>
      <button class="collab-sheet-editor__feature" @click="openNamesModal" type="button" title="命名区域管理" data-testid="sheet-names-btn">命名</button>
      <button class="collab-sheet-editor__feature" @click="openTableModal" type="button" title="插入表对象">表格</button>
      <span class="collab-sheet-editor__peers">
        <span
          v-for="p in peers"
          :key="p.clientId"
          class="collab-sheet-editor__peer"
          :style="{ backgroundColor: p.color }"
          :title="p.displayName"
        >{{ initialOf(p.displayName) }}</span>
      </span>
    </div>
    <!-- Sheet tabs (multi-sheet support) -->
    <div v-if="!loading && sheets.length > 1" class="collab-sheet-editor__tabs">
      <button
        v-for="(sh, si) in sheets"
        :key="si"
        class="collab-sheet-editor__tab"
        :class="{ active: si === activeSheet, hidden: sheetHiddenBySheet[si] }"
        :title="sheetHiddenBySheet[si] ? `${sh.name} (已隐藏)` : sh.name"
        @click="switchSheet(si)"
      >
        {{ sh.name }}
      </button>
      <button class="collab-sheet-editor__tab collab-sheet-editor__add-col" @click="addSheet" type="button" title="新增 sheet">+ 新 sheet</button>
    </div>
    <div v-if="featureDialog" class="collab-sheet-editor__modal-bg" @click="featureDialog = null">
      <div class="collab-sheet-editor__modal" @click.stop>
        <h3 v-if="featureDialog === 'freeze'">冻结窗格 ({{ activeSheetName }})</h3>
        <h3 v-else-if="featureDialog === 'filter'">筛选 ({{ activeSheetName }})</h3>
        <h3 v-else-if="featureDialog === 'cf'">条件格式 ({{ activeSheetName }})</h3>
        <h3 v-else-if="featureDialog === 'dv'">数据验证 ({{ activeSheetName }})</h3>
        <h3 v-else-if="featureDialog === 'spark'">迷你图 ({{ activeSheetName }})</h3>
        <h3 v-else-if="featureDialog === 'pageSetup'">页面布局 ({{ activeSheetName }})</h3>
        <h3 v-else-if="featureDialog === 'sheetManage'">工作表管理</h3>
        <h3 v-else-if="featureDialog === 'note'">单元格批注 ({{ activeSheetName }})</h3>
        <h3 v-else-if="featureDialog === 'hyperlink'">超链接 ({{ activeSheetName }})</h3>
        <h3 v-else-if="featureDialog === 'find'">查找替换 ({{ activeSheetName }})</h3>
        <h3 v-else-if="featureDialog === 'sort'">按列排序 ({{ activeSheetName }})</h3>
        <h3 v-else-if="featureDialog === 'names'">命名区域 (workbook)</h3>
        <h3 v-else-if="featureDialog === 'table'">插入表格对象 ({{ activeSheetName }})</h3>

        <template v-if="featureDialog === 'freeze'">
          <label>冻结上方行数 <input v-model.number="freezeRowsInput" type="number" min="0" max="50" /></label>
          <label>冻结左侧列数 <input v-model.number="freezeColsInput" type="number" min="0" max="20" /></label>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="onFreezeClear">清除</button>
            <button type="button" @click="featureDialog = null">取消</button>
            <button type="button" @click="onFreezeCommit">应用</button>
          </div>
        </template>

        <template v-else-if="featureDialog === 'filter'">
          <label>列号 <input v-model="filterColumnInput" placeholder="例: A2 (列+起始行)" maxlength="6" /></label>
          <label>筛选值 <input v-model="filterValuesInput" placeholder="例: 苹果,香蕉 (逗号或空格分隔)" /></label>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="onFilterClear">清空</button>
            <button type="button" @click="featureDialog = null">取消</button>
            <button type="button" @click="onFilterCommit">应用</button>
          </div>
        </template>

        <template v-else-if="featureDialog === 'cf'">
          <label>单元格 <input v-model="cfColumnInput" placeholder="例: B3" maxlength="5" /></label>
          <label>条件
            <select v-model="cfOperatorInput">
              <option value="greaterThan">大于</option>
              <option value="lessThan">小于</option>
              <option value="equal">等于</option>
              <option value="between">介于</option>
            </select>
          </label>
          <label>阈值 <input v-model="cfValueInput" type="number" /></label>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="onCfClear">清空</button>
            <button type="button" @click="featureDialog = null">取消</button>
            <button type="button" @click="onCfCommit">应用</button>
          </div>
        </template>

        <template v-else-if="featureDialog === 'dv'">
          <label>单元格 <input v-model="dvColumnInput" placeholder="例: B3" maxlength="5" /></label>
          <label>类型
            <select v-model="dvTypeInput">
              <option value="list">下拉列表</option>
              <option value="whole">整数范围</option>
            </select>
          </label>
          <label>允许值 <input v-model="dvValuesInput" :placeholder="dvTypeInput === 'list' ? '例: 是,否' : '例: 1,100'" /></label>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="onDvClear">清空</button>
            <button type="button" @click="featureDialog = null">取消</button>
            <button type="button" @click="onDvCommit">应用</button>
          </div>
        </template>

        <template v-else-if="featureDialog === 'spark'">
          <label>类型
            <select v-model="sparkTypeInput">
              <option value="line">折线</option>
              <option value="column">柱状</option>
              <option value="stacked">盈亏</option>
            </select>
          </label>
          <label>目标单元格 <input v-model="sparkTargetInput" placeholder="例: E2" maxlength="5" /></label>
          <label>源范围 <input v-model="sparkSourceInput" placeholder="例: B2:B10" /></label>
          <label>颜色 <input v-model="sparkColorInput" placeholder="#638EC6" maxlength="7" /></label>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="onSparklineClear">清空</button>
            <button type="button" @click="featureDialog = null">取消</button>
            <button type="button" @click="onSparklineCommit">应用</button>
          </div>
        </template>

        <!-- v0.7.44 — Page Setup modal -->
        <template v-else-if="featureDialog === 'pageSetup'">
          <div class="collab-sheet-editor__modal-body">
            <label>方向：
              <select v-model="pageOrientationInput">
                <option value="portrait">纵向</option>
                <option value="landscape">横向</option>
              </select>
            </label>
            <label>纸张：
              <select v-model.number="pagePaperSizeInput">
                <option :value="1">Letter</option>
                <option :value="9">A4</option>
                <option :value="8">A3</option>
                <option :value="5">Legal</option>
              </select>
            </label>
            <label>页边距：
              <select v-model="pageMarginsInput">
                <option value="normal">标准</option>
                <option value="wide">宽</option>
                <option value="narrow">窄</option>
              </select>
            </label>
            <label>
              <input type="checkbox" v-model="pageFitToPageInput" /> 缩放到一页
            </label>
            <label v-if="pageFitToPageInput">宽度：
              <input type="number" min="1" v-model.number="pageFitToWidthInput" />
            </label>
            <label v-if="pageFitToPageInput">高度：
              <input type="number" min="1" v-model.number="pageFitToHeightInput" />
            </label>
            <label>
              <input type="checkbox" v-model="pageGridlinesInput" /> 打印网格线
            </label>
            <label>
              <input type="checkbox" v-model="pageHeadingsInput" /> 打印行列号
            </label>
            <label>页眉（居中）：
              <input type="text" v-model="pageHeaderCenterInput" placeholder="&amp;C页眉内容" />
            </label>
            <label>页脚（居中）：
              <input type="text" v-model="pageFooterCenterInput" placeholder="&amp;C页脚内容" />
            </label>
            <label>打印区域（A1:F20 留空清除）：
              <input type="text" v-model="pagePrintAreaInput" />
            </label>
          </div>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="onPageSetupClear">清除</button>
            <button type="button" @click="featureDialog = null">取消</button>
            <button type="button" @click="onPageSetupCommit">应用</button>
          </div>
        </template>

        <!-- v0.7.44 — Sheet Manage modal -->
        <template v-else-if="featureDialog === 'sheetManage'">
          <div class="collab-sheet-editor__modal-body">
            <table class="collab-sheet-editor__sheet-manage">
              <thead>
                <tr><th>顺序</th><th>原名</th><th>新名</th><th>可见</th><th></th></tr>
              </thead>
              <tbody>
                <tr v-for="(sh, si) in sheets" :key="si">
                  <td>
                    <button type="button" @click="moveSheetUp(si)" :disabled="si === 0">↑</button>
                    <button type="button" @click="moveSheetDown(si)" :disabled="si === sheets.length - 1">↓</button>
                  </td>
                  <td>{{ sh.name }}</td>
                  <td>
                    <input type="text" v-model="sheetRenameDraftBySheet[si]" :placeholder="sh.name" />
                  </td>
                  <td>
                    <input type="checkbox" :checked="!sheetHiddenBySheet[si]" @change="toggleSheetHidden(si)" />
                  </td>
                  <td>
                    <button type="button" @click="removeSheet(si)" :disabled="sheets.length <= 1">删除</button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="featureDialog = null">关闭</button>
            <button type="button" @click="applySheetRenames">应用改名</button>
          </div>
        </template>

        <!-- v0.7.45 — Note modal -->
        <template v-else-if="featureDialog === 'note'">
          <div class="collab-sheet-editor__modal-body">
            <label>单元格地址：
              <input type="text" v-model="noteRowInput" placeholder="行(数字)" style="width:60px" />
              <input type="text" v-model="noteColInput" placeholder="列(A,B,...)" style="width:60px" />
            </label>
            <label>作者：
              <input type="text" v-model="noteAuthorInput" placeholder="批注作者" />
            </label>
            <label>批注内容：
              <textarea v-model="noteTextInput" rows="3" placeholder="输入批注..."></textarea>
            </label>
            <div v-if="(notesBySheet[activeSheet] || []).length" class="collab-sheet-editor__notes-list">
              <h4>当前批注：</h4>
              <ul>
                <li v-for="(n, ni) in notesBySheet[activeSheet]" :key="ni">
                  {{ cellLabel(n.row, n.column) }} - {{ n.author }}: {{ n.text }}
                </li>
              </ul>
            </div>
          </div>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="onNoteClear">清除</button>
            <button type="button" @click="featureDialog = null">取消</button>
            <button type="button" @click="onNoteCommit">添加</button>
          </div>
        </template>

        <!-- v0.7.45 — Hyperlink modal -->
        <template v-else-if="featureDialog === 'hyperlink'">
          <div class="collab-sheet-editor__modal-body">
            <label>单元格地址：
              <input type="text" v-model="linkRowInput" placeholder="行(数字)" style="width:60px" />
              <input type="text" v-model="linkColInput" placeholder="列(A,B,...)" style="width:60px" />
            </label>
            <label>目标（URL / #Sheet!A1）：
              <input type="text" v-model="linkTargetInput" placeholder="https://... 或 #Sheet2!A1" />
            </label>
            <div v-if="(hyperlinksBySheet[activeSheet] || []).length" class="collab-sheet-editor__hyperlinks-list">
              <h4>当前超链接：</h4>
              <ul>
                <li v-for="(h, hi) in hyperlinksBySheet[activeSheet]" :key="hi">
                  {{ cellLabel(h.row, h.column) }} → {{ h.target }}
                </li>
              </ul>
            </div>
          </div>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="onHyperlinkClear">清除</button>
            <button type="button" @click="featureDialog = null">取消</button>
            <button type="button" @click="onHyperlinkCommit">添加</button>
          </div>
        </template>

        <!-- v0.7.99 — Find & replace modal -->
        <template v-else-if="featureDialog === 'find'">
          <div class="collab-sheet-editor__modal-body">
            <label>查找内容：
              <input
                type="text"
                v-model="findSearchInput"
                placeholder="例如: hello"
                data-testid="sheet-find-search"
              />
            </label>
            <label>替换为：
              <input
                type="text"
                v-model="findReplaceInput"
                placeholder="替换文本（留空 = 仅查找）"
                data-testid="sheet-find-replace"
              />
            </label>
            <label>
              <input type="checkbox" v-model="findMatchCaseInput" data-testid="sheet-find-case" />
              区分大小写
            </label>
            <div class="collab-sheet-editor__find-summary" data-testid="sheet-find-summary">
              {{ findMatches.length }} 个匹配
            </div>
            <ul v-if="findMatches.length" class="collab-sheet-editor__find-list" data-testid="sheet-find-list">
              <li v-for="(m, mi) in findMatches.slice(0, 20)" :key="mi">
                {{ cellLabel(m.row, m.column) }}：{{ m.before }}
              </li>
              <li v-if="findMatches.length > 20">… 还有 {{ findMatches.length - 20 }} 个</li>
            </ul>
          </div>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="onFindClear">清空查找</button>
            <button type="button" @click="featureDialog = null">关闭</button>
            <button type="button" @click="onFindCommit" :disabled="!findSearchInput || !findMatches.length" data-testid="sheet-find-replace-btn">全部替换</button>
          </div>
        </template>

        <!-- v0.7.100 — Sort modal -->
        <template v-else-if="featureDialog === 'sort'">
          <div class="collab-sheet-editor__modal-body">
            <label>排序依据列：
              <input type="text" v-model="sortColInput" placeholder="列(A,B,...)" style="width:80px" data-testid="sheet-sort-col" />
            </label>
            <label>方向：
              <select v-model="sortDirectionInput" data-testid="sheet-sort-direction">
                <option value="asc">升序 (A→Z, 0→9)</option>
                <option value="desc">降序 (Z→A, 9→0)</option>
              </select>
            </label>
            <label>起始行：
              <input type="number" v-model.number="sortStartRowInput" :min="1" :max="rows.length" style="width:60px" data-testid="sheet-sort-start-row" />
            </label>
            <label>结束行：
              <input type="number" v-model.number="sortEndRowInput" :min="1" :max="rows.length" style="width:60px" data-testid="sheet-sort-end-row" />
            </label>
            <p class="collab-sheet-editor__sort-hint">第 {{ sortStartRowInput }} - {{ sortEndRowInput }} 行将按 {{ sortColInput || '?' }} 列{{ sortDirectionInput === 'desc' ? '降序' : '升序' }}排列。</p>
          </div>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="featureDialog = null">取消</button>
            <button type="button" @click="onSortCommit" :disabled="sortColInput.trim() === '' || colToIndex(sortColInput.trim()) < 0" data-testid="sheet-sort-apply-btn">应用排序</button>
          </div>
        </template>

        <!-- v0.7.102 — workbook-level Named Ranges (Excel Defined Names) -->
        <template v-else-if="featureDialog === 'names'">
          <div class="collab-sheet-editor__modal-body">
            <ul v-if="definedNamesList.length" class="collab-sheet-editor__names-list" data-testid="sheet-names-list">
              <li v-for="(entry, idx) in definedNamesList" :key="idx">
                <code class="collab-sheet-editor__names-name">{{ entry.name }}</code>
                <span class="collab-sheet-editor__names-formula">= {{ entry.formula }}{{ entry.sheetIndex !== undefined ? ` (sheet #${entry.sheetIndex})` : ' (workbook)' }}</span>
                <button type="button" class="collab-sheet-editor__names-del" @click="deleteDefinedName(idx)" :data-testid="`sheet-names-del-${idx}`">删除</button>
              </li>
            </ul>
            <p v-else class="collab-sheet-editor__names-empty" data-testid="sheet-names-empty">暂无命名区域。添加一个：</p>
            <!-- v0.7.105 — Create-from-selection: auto-fill formula from the
                 currently-selected cell range. Disabled until at least one cell
                 is selected. -->
            <button
              type="button"
              class="collab-sheet-editor__names-from-selection"
              :disabled="!selectedRangeA1"
              @click="addDefinedNameFromSelection"
              data-testid="sheet-names-from-selection"
              title="基于当前选中的单元格范围创建一个命名区域"
            >⊞ 从选区创建 ({{ selectedRangeA1 || '请先选择范围' }})</button>
            <div class="collab-sheet-editor__names-add">
              <label>名称：<input type="text" v-model="newDefinedName" placeholder="MyRange" data-testid="sheet-names-input-name" /></label>
              <label>公式 (A1)：<input type="text" v-model="newDefinedFormula" placeholder="Sheet1!$A$1:$D$10" data-testid="sheet-names-input-formula" /></label>
              <label>范围：
                <select v-model.number="newDefinedScope" data-testid="sheet-names-input-scope">
                  <option :value="-1">workbook (全局)</option>
                  <option v-for="(s, si) in sheets" :key="si" :value="si">{{ s.name }} (sheet #{{ si }})</option>
                </select>
              </label>
              <button type="button" @click="addDefinedName" :disabled="!newDefinedName.trim() || !newDefinedFormula.trim()" data-testid="sheet-names-add-btn">添加</button>
            </div>
          </div>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="featureDialog = null">关闭</button>
          </div>
        </template>

        <!-- v0.7.45 — Table modal -->
        <template v-else-if="featureDialog === 'table'">
          <div class="collab-sheet-editor__modal-body">
            <label>表名：
              <input type="text" v-model="tableNameInput" placeholder="SalesTable" />
            </label>
            <label>起始行：
              <input type="text" v-model="tableStartRowInput" placeholder="1" style="width:60px" />
              起始列：
              <input type="text" v-model="tableStartColInput" placeholder="A" style="width:60px" />
            </label>
            <label>结束行：
              <input type="text" v-model="tableEndRowInput" placeholder="3" style="width:60px" />
              结束列：
              <input type="text" v-model="tableEndColInput" placeholder="C" style="width:60px" />
            </label>
            <label>列名（逗号分隔）：
              <input type="text" v-model="tableColumnsInput" placeholder="Q1, Q2, Q3" />
            </label>
            <div v-if="(tablesBySheet[activeSheet] || []).length" class="collab-sheet-editor__tables-list">
              <h4>当前表对象：</h4>
              <ul>
                <li v-for="(t, ti) in tablesBySheet[activeSheet]" :key="ti">
                  {{ t.name }} ({{ t.area.startRow }}-{{ t.area.endRow }})
                </li>
              </ul>
            </div>
          </div>
          <div class="collab-sheet-editor__modal-actions">
            <button type="button" @click="onTableClear">清除</button>
            <button type="button" @click="featureDialog = null">取消</button>
            <button type="button" @click="onTableCommit">添加</button>
          </div>
        </template>
      </div>
    </div>

    <div v-if="loading" class="collab-sheet-editor__loading">加载表格中...</div>
    <!-- Formula bar -->
    <div v-if="!loading" class="collab-sheet-editor__formula">
      <span class="collab-sheet-editor__cellref">{{ selectedRef || '选单元格' }}</span>
      <span class="collab-sheet-editor__fx">fx</span>
      <input
        class="collab-sheet-editor__formula-input"
        :value="formulaValue"
        @input="formulaValue = ($event.target as HTMLInputElement).value; formulaError = null"
        @keydown.enter.exact="commitFormula"
        @keydown.tab.exact.prevent="acceptNameSuggestion(nameSuggestActiveIdx)"
        @keydown.down.exact.prevent="nameSuggestActiveIdx = (nameSuggestActiveIdx + 1) % Math.max(nameSuggestions.length, 1)"
        @keydown.up.exact.prevent="nameSuggestActiveIdx = (nameSuggestActiveIdx + nameSuggestions.length - 1) % Math.max(nameSuggestions.length, 1)"
        placeholder="输入公式 (例: =SUM(A1:A10))"
      />
      <span v-if="formulaError" class="collab-sheet-editor__formula-error">⚠ {{ formulaError }}</span>
      <span v-else-if="formulaValue" class="collab-sheet-editor__formula-hint">= {{ formulaResult }}</span>
      <!-- v0.7.105 — defined-name autocomplete popup under the formula bar -->
      <ul
        v-if="nameSuggestions.length"
        class="collab-sheet-editor__name-suggest"
        data-testid="sheet-formula-name-suggest"
      >
        <li
          v-for="(s, idx) in nameSuggestions"
          :key="s.name + (s.sheetIndex ?? -1)"
          :class="{ 'collab-sheet-editor__name-suggest--active': idx === nameSuggestActiveIdx }"
          :data-testid="`sheet-formula-name-suggest-${idx}`"
          @mousedown.prevent="acceptNameSuggestion(idx)"
        ><code>{{ s.name }}</code><span class="collab-sheet-editor__name-suggest-formula">= {{ s.formula }}{{ s.sheetIndex !== undefined ? ` (sheet #${s.sheetIndex})` : '' }}</span></li>
      </ul>
    </div>
    <div v-if="!loading" class="collab-sheet-editor__table-wrap">
      <table class="collab-sheet-editor__grid">
        <thead>
          <tr>
            <th class="collab-sheet-editor__rowhead"></th>
            <th v-for="(col, ci) in cols" :key="ci" class="collab-sheet-editor__colhead">
              <input
                class="collab-sheet-editor__header-input"
                :value="col"
                @input="renameColumn(ci, ($event.target as HTMLInputElement).value)"
              />
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(row, ri) in rows" :key="ri">
            <th class="collab-sheet-editor__rowhead">{{ ri + 1 }}</th>
            <td v-for="(col, ci) in cols" :key="ci">
              <div
                v-if="isCellLocked(ri, ci)"
                class="collab-sheet-editor__cell-lock"
                :style="{ color: cellLocker(ri, ci)?.color }"
                :title="`正在编辑：${cellLocker(ri, ci)?.displayName}`"
                :data-testid="`sheet-cell-lock-${ri}-${ci}`"
              >
                🔒
              </div>
              <div
                v-if="remoteCellPeer(ri, ci)"
                class="collab-sheet-editor__peer-label"
                :style="{
                  background: remoteCellPeer(ri, ci)?.color,
                  borderColor: remoteCellPeer(ri, ci)?.color,
                }"
                :data-testid="`sheet-peer-label-${ri}-${ci}`"
              >
                {{ remoteCellPeer(ri, ci)?.displayName }}
              </div>
              <input
                class="collab-sheet-editor__cell-input"
                :class="{
                  'is-formula': !!cellFormula(ri, ci),
                  'is-percent': cellPercent(ri, ci),
                  'collab-sheet-editor__cell--selected': selectedRi === ri && selectedCi === ci,
                  'collab-sheet-editor__cell--remote': remoteCellPeer(ri, ci),
                  'collab-sheet-editor__cell--locked': isCellLocked(ri, ci),
                }"
                :style="remoteCellStyle(ri, ci)"
                :value="cellFormula(ri, ci) || cellText(ri, ci)"
                :title="cellFormula(ri, ci) || (isCellLocked(ri, ci) ? `🔒 ${cellLocker(ri, ci)?.displayName} 正在编辑` : '')"
                :readonly="isCellLocked(ri, ci)"
                :data-cell="`${ri}-${ci}`"
                @focus="(e: FocusEvent) => onCellSelect(ri, ci, (e as any)?.shiftKey)
                "
                @click="(e: MouseEvent) => onCellSelect(ri, ci, e?.shiftKey)
                "
                @input="setCell(ri, ci, ($event.target as HTMLInputElement).value)"
              />
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-if="error || saveError" class="collab-sheet-editor__error">{{ saveError || error }}</p>
  

    <!-- v0.7.38 — sheet comment panel (cell-level anchor). -->
    <CollabCommentsPanel
      :doc-id="docId"
      :token="token"
      :anchor="commentAnchor"
      anchor-label="单元格"
      placeholder="对选中的单元格添加评论…"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import * as Y from 'yjs'
import CollabCommentsPanel from '@/components/collab/CollabCommentsPanel.vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useYjsCollabDoc } from '@/composables/useYjsCollabDoc'
import {
  openXlsx,
  saveXlsxBytes,
  newXlsxWorkbook,
  type XlsxAdapterWorkbook,
  type XlsxAdapterCell,
  parseDefinedNames,
} from '@/editor/adapters/xlsxAdapter'
import type { DefinedNameEntry } from '@/editor/adapters/xlsxDefinedNames'
import {
  downloadCollabDocBytes,
  uploadCollabDocBytes,
} from '@/api/collabDoc'
import {
  evaluateFormula as evalFormula,
  type SheetLookup,
} from '@/editor/formula/sheetFormula'
import {
  applyFilterState,
  type SheetFilterState,
} from '@/editor/adapters/xlsxFilter'
import { applyCfRules, type CfWireRule } from '@/editor/adapters/xlsxCf'
import { buildLockMap, cellKey, checkEditAllowed, type RemoteCellPeer } from '@/editor/adapters/xlsxCellLock'
import { applyDvRules, type DvWireRule } from '@/editor/adapters/xlsxDv'
import { applySparklineAdditions, type SparklineGroupAdd } from '@/editor/adapters/xlsxSparkline'
import { transformWorkbook, transformPackage, inspectXlsx, type MutablePackage } from '@/editor/adapters/xlsxWorksheetIo'
import { applyPageSetupState, type SheetPageSetupState } from '@/editor/adapters/xlsxPageSetup'
import { applyHyperlinkEdits, type HyperlinkEdit } from '@/editor/adapters/xlsxHyperlinks'
import { applySheetNotes, type SheetNote } from '@/editor/adapters/xlsxNotes'
import { applyTableAdditions, type TableAddition } from '@/editor/adapters/xlsxTableAdd'

const props = defineProps<{
  docId: string
  title: string
  token: string
  displayName: string
}>()

const connected = ref(false)
const peers = ref<Array<{ clientId: number; displayName: string; color: string }>>([])
const error = ref<string | null>(null)
const cols = ref<string[]>(['A', 'B', 'C'])
const sheets = ref<Array<{ name: string; rows: string[][] }>>([])
const activeSheet = ref(0)
const activeSheetName = computed(() => {
  return sheets.value[activeSheet.value]?.name || ('Sheet' + (activeSheet.value + 1))
})
// Selected cell tracking for the formula bar
const selectedRi = ref(-1)
const selectedCi = ref(-1)
// v0.7.105 — range selection state for shift-click multi-select and
// "从选区创建" / defined-name ranges. Anchored at the first click,
// the end is moved by subsequent shift-clicks.
const rangeAnchorRi = ref(-1)
const rangeAnchorCi = ref(-1)
const rangeEndRi = ref(-1)
const rangeEndCi = ref(-1)
const formulaValue = ref('')
const formulaError = ref<string | null>(null)

// v0.7.43 — per-sheet SHEET feature state (Feishu parity: freeze, filter,
// conditional formatting, data validation, sparklines). Each record is
// applied to the worksheet XML in flushSave via transformWorkbook.
const freezeBySheet = ref<Record<number, { rows: number; cols: number } | null>>({})
const filterBySheet = ref<Record<number, SheetFilterState | null>>({})
const cfBySheet = ref<Record<number, CfWireRule[]>>({})
const dvBySheet = ref<Record<number, DvWireRule[]>>({})
const sparkBySheet = ref<Record<number, SparklineGroupAdd[]>>({})
const pageSetupBySheet = ref<Record<number, SheetPageSetupState | null>>({})
const sheetHiddenBySheet = ref<Record<number, boolean>>({})
const sheetRenameDraftBySheet = ref<Record<number, string>>({})
// v0.7.105 — track the sheet names from the most recent successful save so
// flushSave can build the renamedSheets map for the xlsxAdapter (which then
// rewrites <definedName> formulas to follow renames).
const lastSavedSheetNames = ref<string[]>([])
const notesBySheet = ref<Record<number, SheetNote[]>>({})
const hyperlinksBySheet = ref<Record<number, HyperlinkEdit[]>>({})
const tablesBySheet = ref<Record<number, TableAddition[]>>({})
const featureDialog = ref<null | 'freeze' | 'filter' | 'cf' | 'dv' | 'spark' | 'pageSetup' | 'sheetManage' | 'note' | 'hyperlink' | 'find' | 'sort' | 'table' | 'names'>(null)

// Modal-input reactive state — re-used across all 5 modals.
const freezeRowsInput = ref(1)
const freezeColsInput = ref(1)
const filterColumnInput = ref('')
const filterValuesInput = ref('')
const cfColumnInput = ref('')
const cfOperatorInput = ref<'greaterThan' | 'lessThan' | 'equal' | 'between'>('greaterThan')
const cfValueInput = ref('')
const dvColumnInput = ref('')
const dvTypeInput = ref<'list' | 'whole'>('list')
const dvValuesInput = ref('')
const sparkTargetInput = ref('')
const sparkSourceInput = ref('')
const sparkColorInput = ref('#638EC6')
const sparkTypeInput = ref<'line' | 'column' | 'stacked'>('column')

// v0.7.44 — page setup modal inputs (per active sheet)
const pageOrientationInput = ref<'portrait' | 'landscape'>('portrait')
const pagePaperSizeInput = ref<number>(1)
const pageMarginsInput = ref<'normal' | 'wide' | 'narrow'>('normal')
const pageGridlinesInput = ref(false)
const pageHeadingsInput = ref(false)
const pageFitToWidthInput = ref<number>(1)
const pageFitToHeightInput = ref<number>(1)
const pageFitToPageInput = ref(false)
const pageHeaderCenterInput = ref('')
const pageFooterCenterInput = ref('')
const pagePrintAreaInput = ref('')

// v0.7.45 — Note modal inputs
const noteRowInput = ref('')
const noteColInput = ref('')
const noteAuthorInput = ref('')
const noteTextInput = ref('')

// v0.7.45 — Hyperlink modal inputs
const linkRowInput = ref('')
// v0.7.99 — find & replace inputs
const findSearchInput = ref('')
const findReplaceInput = ref('')
const findMatchCaseInput = ref(false)
// v0.7.100 — sort inputs
const sortColInput = ref('A')
const sortDirectionInput = ref<'asc' | 'desc'>('asc')
const sortStartRowInput = ref(1)
const sortEndRowInput = ref(0)
// v0.7.102 — workbook-level named ranges modal state
const newDefinedName = ref('')
const newDefinedFormula = ref('')
const newDefinedScope = ref<-1 | number>(-1)
const linkColInput = ref('')
const linkTargetInput = ref('')

// v0.7.45 — Table modal inputs
const tableNameInput = ref('')
const tableStartRowInput = ref('1')
const tableStartColInput = ref('A')
const tableEndRowInput = ref('3')
const tableEndColInput = ref('C')
const tableColumnsInput = ref('Q1, Q2, Q3')

// v0.7.43 — formula engine moved to editor/formula/sheetFormula.ts (cross-sheet
// refs, IF / COUNTIF / SUMIF / VLOOKUP / CONCAT / LEN / ROUND / ABS / TEXT,
// and token-level arithmetic).
const sheetLookup = computed<SheetLookup>(() => {
  const m = new Map<string, string[][]>()
  for (const sh of sheets.value) m.set(sh.name, sh.rows)
  return m
})

const selectedRef = computed(() =>
  selectedRi.value >= 0 && selectedCi.value >= 0
    ? `${colName(selectedCi.value)}${selectedRi.value + 1}`
    : '',
)
const formulaResult = computed(() => {
  if (!formulaValue.value.startsWith('=')) return ''
  try {
    return evalFormula(formulaValue.value, activeSheetName.value, sheetLookup.value)
  } catch (e: any) {
    return ''
  }
})

// v0.7.105 — name suggestions. When the user types `=`, we extract the
// trailing identifier token (e.g. "=SU" -> "SU") and find every defined
// name whose first letters match (case-insensitive prefix match).
const nameSuggestToken = computed(() => {
  const v = formulaValue.value
  if (!v.startsWith('=')) return ''
  // Match the partial identifier right after the leading "=" (and any
  // preceding function / sheet / cell prefixes the user may have already
  // committed with Enter). The simplest UX is to match the last identifier
  // chunk: letters/digits/_/. preceded by an operator or the start.
  const m = /([A-Za-z_\x5c][A-Za-z0-9_.\x5c]*)$/.exec(v.slice(1))
  return m ? m[1] : ''
})
const nameSuggestActiveIdx = ref(0)
const nameSuggestions = computed(() => {
  const tok = nameSuggestToken.value
  if (!tok) return []
  const lower = tok.toLowerCase()
  return definedNamesList.value.filter((e) => e.name.toLowerCase().startsWith(lower)).slice(0, 8)
})
watch(nameSuggestions, () => { nameSuggestActiveIdx.value = 0 })
const acceptNameSuggestion = (idx: number) => {
  const list = nameSuggestions.value
  const pick = list[idx]
  if (!pick) return
  // Replace the partial token with the full name.
  const cur = formulaValue.value
  const tok = nameSuggestToken.value
  const replaced = cur.slice(0, cur.length - tok.length) + pick.name
  formulaValue.value = replaced
  nameSuggestActiveIdx.value = 0
  formulaError.value = null
}
// v0.7.38 — remote cell-selection highlighter (peer awareness)
const remoteCellPeer = (ri: number, ci: number) => {
  return remoteSelections.value.find((p) => p.cell && p.cell.ri === ri && p.cell.ci === ci) || null
}
const remoteCellStyle = (ri: number, ci: number) => {
  const p = remoteCellPeer(ri, ci)
  if (!p) return {}
  return { outline: `2px solid ${p.color}`, outlineOffset: '-1px' }
}

// v0.7.77 — soft optimistic cell lock (read-lock from awareness selection)
const myClientId = computed(() => handle?.provider?.awareness?.clientID ?? -1)
const lockMap = computed(() => buildLockMap(remoteSelections.value, myClientId.value))
const isCellLocked = (ri: number, ci: number) => lockMap.value.has(cellKey(ri, ci))
const cellLocker = (ri: number, ci: number): RemoteCellPeer | null =>
  lockMap.value.get(cellKey(ri, ci)) ?? null

// v0.7.38 — sheet comment anchor (cell-level)
const commentAnchor = ref<{ type: 'sheet'; ref: string } | null>(null)

const cellLabel = (ri: number, ci: number) => {
  // Spreadsheet column letter (A, B, ..., Z, AA, ...)
  let n = ci
  let s = ''
  do {
    s = String.fromCharCode(65 + (n % 26)) + s
    n = Math.floor(n / 26) - 1
  } while (n >= 0)
  return `${s}${ri + 1}`
}

// v0.7.105 — normalized selection range as {r1,c1,r2,c2} (top-left → bottom-right).
const selectedRange = computed(() => {
  if (rangeAnchorRi.value < 0 || rangeAnchorCi.value < 0) return null
  if (rangeEndRi.value < 0 || rangeEndCi.value < 0) return null
  const r1 = Math.min(rangeAnchorRi.value, rangeEndRi.value)
  const c1 = Math.min(rangeAnchorCi.value, rangeEndCi.value)
  const r2 = Math.max(rangeAnchorRi.value, rangeEndRi.value)
  const c2 = Math.max(rangeAnchorCi.value, rangeEndCi.value)
  return { r1, c1, r2, c2 }
})

/** v0.7.105 — A1 notation for the current range, e.g. "Sheet1!$A$1:$D$10". */
const selectedRangeA1 = computed(() => {
  const r = selectedRange.value
  if (!r) return ''
  const sheetName = sheets.value[activeSheet.value]?.name || ('Sheet' + (activeSheet.value + 1))
  const escapedSheet = /[^A-Za-z0-9_]/.test(sheetName) ? `'${sheetName.replace(/'/g, "''")}'` : sheetName
  return `${escapedSheet}!$${cellLabel(r.r1, r.c1)}:$${cellLabel(r.r2, r.c2)}`
})

const onCellSelect = (ri: number, ci: number, shiftKey: boolean = false) => {
  // v0.7.105 — shift-click extends the range instead of moving the anchor.
  // The first click sets the anchor + end to the same cell; subsequent
  // shift-clicks move only the end so selectedRange tracks the union.
  if (shiftKey && rangeAnchorRi.value >= 0 && rangeAnchorCi.value >= 0) {
    rangeEndRi.value = ri
    rangeEndCi.value = ci
    selectedRi.value = ri
    selectedCi.value = ci
    formulaValue.value = ''
    return
  }
  rangeAnchorRi.value = ri
  rangeAnchorCi.value = ci
  rangeEndRi.value = ri
  rangeEndCi.value = ci
  selectedRi.value = ri
  selectedCi.value = ci
  const raw = rows.value[ri]?.[ci]
  formulaValue.value = raw && raw.startsWith('=') ? raw : ''
  formulaError.value = null
  commentAnchor.value = { type: 'sheet', ref: cellLabel(ri, ci) }
  // v0.7.38 — broadcast cell selection on the Yjs awareness layer
  // so collaborators see the active cell outline in real time.
  try {
    handle?.publishCellSelection?.(ri, ci)
  } catch (e) {
    // eslint-disable-next-line no-console
    console.warn('[CollabSheetEditor] publishCellSelection failed', e)
  }
}
const commitFormula = () => {
  if (selectedRi.value < 0 || selectedCi.value < 0) return
  setCell(selectedRi.value, selectedCi.value, formulaValue.value)
  formulaError.value = null
}
const switchSheet = (i: number) => {
  if (i < 0 || i >= sheets.value.length) return
  // Persist current edits into sheets[]
  if (sheets.value[activeSheet.value]) {
    sheets.value[activeSheet.value] = {
      name: sheets.value[activeSheet.value].name,
      rows: rows.value.map((r) => r.slice()),
    }
  }
  activeSheet.value = i
  rows.value = sheets.value[i].rows.map((r) => r.slice())
  cols.value = Array.from({ length: Math.max(rows.value[0]?.length || 0, 1) }, (_, k) => colName(k))
  selectedRi.value = -1
  selectedCi.value = -1
  formulaValue.value = ''
}

const commitSheetManifest = () => {
  if (!ysheetSnapshot || !handle) return
  const names = JSON.stringify(sheets.value.map((sheet) => sheet.name))
  handle.ydoc.transact(() => {
    ysheetSnapshot?.set('names', names)
  })
}

const addSheet = () => {
  if (sheets.value[activeSheet.value]) {
    sheets.value[activeSheet.value] = {
      name: sheets.value[activeSheet.value].name,
      rows: rows.value.map((r) => r.slice()),
    }
  }
  const name = 'Sheet' + (sheets.value.length + 1)
  sheets.value.push({ name, rows: [Array.from({ length: cols.value.length }, () => '')] })
  // v0.7.93 -- broadcast sheet addition so collaborators see the new tab.
  if (ysheets && handle) {
    handle.ydoc.transact(() => {
      if (ysheets) ysheets.push([name])
    })
  }
  commitSheetManifest()
  activeSheet.value = sheets.value.length - 1
  rows.value = sheets.value[activeSheet.value].rows.map((r) => r.slice())
  selectedRi.value = -1
  selectedCi.value = -1
  formulaValue.value = ''
  scheduleSave()
}

// v0.7.43 — SHEET feature modal actions. All handlers write into the
// per-sheet reactive ref, then trigger scheduleSave() so flushSave picks
// them up via transformWorkbook.
const openFreezeModal = () => {
  freezeRowsInput.value = freezeBySheet.value[activeSheet.value]?.rows ?? 1
  freezeColsInput.value = freezeBySheet.value[activeSheet.value]?.cols ?? 1
  featureDialog.value = 'freeze'
}
const onFreezeCommit = () => {
  freezeBySheet.value[activeSheet.value] = {
    rows: Math.max(0, Number(freezeRowsInput.value) || 0),
    cols: Math.max(0, Number(freezeColsInput.value) || 0),
  }
  featureDialog.value = null
  scheduleSave()
}
const onFreezeClear = () => {
  freezeBySheet.value[activeSheet.value] = null
  featureDialog.value = null
  scheduleSave()
}

const openFilterModal = () => {
  filterColumnInput.value = ''
  filterValuesInput.value = ''
  featureDialog.value = 'filter'
}
const onFilterCommit = () => {
  const colStr = filterColumnInput.value.trim().toUpperCase()
  const m = /^([A-Z]+)(\d+)$/.exec(colStr)
  if (!m) {
    MessagePlugin.error('列号格式不正确,例如 A2')
    return
  }
  const values = filterValuesInput.value.split(/[,\s]+/).map((v) => v.trim()).filter(Boolean)
  const sheetName = sheets.value[activeSheet.value]?.name || ('Sheet' + (activeSheet.value + 1))
  let colIdx = 0
  for (const ch of m[1]) colIdx = colIdx * 26 + (ch.charCodeAt(0) - 64)
  colIdx -= 1
  const row = Number(m[2])
  const lastCol = colIdx
  const lastRow = Math.max(rows.value.length, 1)
  filterBySheet.value[activeSheet.value] = {
    sheetName,
    filter: {
      range: { startRow: row - 1, startColumn: colIdx, endRow: lastRow, endColumn: lastCol },
      columns: values.length ? [{ colId: colIdx, values }] : [],
    },
    hiddenRows: [],
    visibilityRange: { startRow: 0, startColumn: 0, endRow: lastRow, endColumn: lastCol },
  }
  featureDialog.value = null
  scheduleSave()
}
const onFilterClear = () => {
  filterBySheet.value[activeSheet.value] = null
  featureDialog.value = null
  scheduleSave()
}

const openCfModal = () => {
  cfColumnInput.value = ''
  cfOperatorInput.value = 'greaterThan'
  cfValueInput.value = ''
  featureDialog.value = 'cf'
}
const onCfCommit = () => {
  const colStr = cfColumnInput.value.trim().toUpperCase()
  if (!/^[A-Z]+\d+$/.test(colStr)) {
    MessagePlugin.error('列号格式不正确,例如 B3')
    return
  }
  const value = Number(cfValueInput.value)
  if (Number.isNaN(value)) {
    MessagePlugin.error('阈值必须是数字')
    return
  }
  cfBySheet.value[activeSheet.value] = [{
    ranges: [{ startRow: 0, endRow: rows.value.length - 1, startColumn: 0, endColumn: cols.value.length - 1 }],
    stopIfTrue: false,
    rule: {
      type: 'highlightCell',
      subType: 'number',
      operator: cfOperatorInput.value,
      value,
      style: { font: { color: 'FF9C0006' }, fill: { bgColor: 'FFFFC7CE' } },
      priority: 1,
      sqref: colStr,
    },
  }]
  featureDialog.value = null
  scheduleSave()
}
const onCfClear = () => {
  cfBySheet.value[activeSheet.value] = []
  featureDialog.value = null
  scheduleSave()
}

const openDvModal = () => {
  dvColumnInput.value = ''
  dvTypeInput.value = 'list'
  dvValuesInput.value = ''
  featureDialog.value = 'dv'
}
const onDvCommit = () => {
  const colStr = dvColumnInput.value.trim().toUpperCase()
  if (!/^[A-Z]+\d+$/.test(colStr)) {
    MessagePlugin.error('列号格式不正确,例如 B3')
    return
  }
  const vals = dvValuesInput.value.split(/[,\s]+/).map((v) => v.trim()).filter(Boolean)
  if (!vals.length) {
    MessagePlugin.error('请至少输入一个有效值')
    return
  }
  dvBySheet.value[activeSheet.value] = [{
    ranges: [{ startRow: 0, endRow: rows.value.length - 1, startColumn: 0, endColumn: cols.value.length - 1 }],
    rule: {
      type: dvTypeInput.value,
      operator: dvTypeInput.value === 'whole' ? 'between' : undefined,
      formula1: dvTypeInput.value === 'list' ? `"${vals.join(',')}"` : String(Number(vals[0])),
      formula2: dvTypeInput.value === 'whole' ? String(Number(vals[1] ?? vals[0])) : undefined,
      sqref: colStr,
    },
  }]
  featureDialog.value = null
  scheduleSave()
}
const onDvClear = () => {
  dvBySheet.value[activeSheet.value] = []
  featureDialog.value = null
  scheduleSave()
}

const openSparkModal = () => {
  sparkTypeInput.value = 'column'
  sparkTargetInput.value = ''
  sparkSourceInput.value = ''
  sparkColorInput.value = '#638EC6'
  featureDialog.value = 'spark'
}
const onSparklineCommit = () => {
  const target = sparkTargetInput.value.trim().toUpperCase()
  const source = sparkSourceInput.value.trim()
  if (!/^[A-Z]+\d+$/.test(target)) {
    MessagePlugin.error('目标单元格格式不正确,例如 E2')
    return
  }
  if (!/^[A-Z]+\d+:[A-Z]+\d+$/i.test(source)) {
    MessagePlugin.error('源范围格式不正确,例如 B2:B10')
    return
  }
  const sheetName = sheets.value[activeSheet.value]?.name || ('Sheet' + (activeSheet.value + 1))
  sparkBySheet.value[activeSheet.value] = (sparkBySheet.value[activeSheet.value] || []).concat([{
    type: sparkTypeInput.value,
    color: sparkColorInput.value,
    cells: [{ cell: target, sourceRef: `${sheetName}!${source}` }],
  }])
  featureDialog.value = null
  scheduleSave()
}
const onSparklineClear = () => {
  sparkBySheet.value[activeSheet.value] = []
  featureDialog.value = null
  scheduleSave()
}

// ===== v0.7.44 — Page Setup modal handlers =====
const openPageSetupModal = () => {
  const existing = pageSetupBySheet.value[activeSheet.value]
  pageOrientationInput.value = existing?.orientation ?? 'portrait'
  pagePaperSizeInput.value = existing?.paperSize ?? 1
  pageMarginsInput.value = existing?.margins ?? 'normal'
  pageGridlinesInput.value = existing?.printGridlines ?? false
  pageHeadingsInput.value = existing?.printHeadings ?? false
  pageFitToPageInput.value = existing?.fitToPage ?? false
  pageFitToWidthInput.value = existing?.fitToWidth ?? 1
  pageFitToHeightInput.value = existing?.fitToHeight ?? 1
  pageHeaderCenterInput.value = existing?.header?.center ?? ''
  pageFooterCenterInput.value = existing?.footer?.center ?? ''
  pagePrintAreaInput.value = existing?.printArea ?? ''
  featureDialog.value = 'pageSetup'
}

const onPageSetupCommit = () => {
  const sheetName = sheets.value[activeSheet.value]?.name || ('Sheet' + (activeSheet.value + 1))
  const headerCenter = pageHeaderCenterInput.value.trim()
  const footerCenter = pageFooterCenterInput.value.trim()
  const printAreaRaw = pagePrintAreaInput.value.trim()
  pageSetupBySheet.value[activeSheet.value] = {
    sheetName,
    orientation: pageOrientationInput.value,
    paperSize: pagePaperSizeInput.value,
    margins: pageMarginsInput.value,
    printGridlines: pageGridlinesInput.value || undefined,
    printHeadings: pageHeadingsInput.value || undefined,
    fitToPage: pageFitToPageInput.value || undefined,
    fitToWidth: pageFitToPageInput.value ? pageFitToWidthInput.value : undefined,
    fitToHeight: pageFitToPageInput.value ? pageFitToHeightInput.value : undefined,
    header: headerCenter ? { center: headerCenter } : null,
    footer: footerCenter ? { center: footerCenter } : null,
    printArea: printAreaRaw || null,
  }
  featureDialog.value = null
  scheduleSave()
}

const onPageSetupClear = () => {
  pageSetupBySheet.value[activeSheet.value] = null
  featureDialog.value = null
  scheduleSave()
}

// ===== v0.7.45 — Note modal handlers =====
const openNoteModal = () => {
  noteRowInput.value = ''
  noteColInput.value = ''
  noteAuthorInput.value = ''
  noteTextInput.value = ''
  featureDialog.value = 'note'
}
const onNoteCommit = () => {
  const row = Number(noteRowInput.value) - 1
  const col = colToIndex(noteColInput.value.trim())
  if (Number.isNaN(row) || row < 0 || col < 0) {
    MessagePlugin.error('单元格格式不正确,例如 行=1 列=A')
    return
  }
  const author = (noteAuthorInput.value || '匿名').trim()
  const text = (noteTextInput.value || '').trim()
  if (!text) {
    MessagePlugin.error('批注内容不能为空')
    return
  }
  const existing = notesBySheet.value[activeSheet.value] || []
  notesBySheet.value[activeSheet.value] = existing.concat([{ row, column: col, author, text }])
  noteRowInput.value = ''
  noteColInput.value = ''
  noteAuthorInput.value = ''
  noteTextInput.value = ''
  scheduleSave()
}
const onNoteClear = () => {
  notesBySheet.value[activeSheet.value] = []
  featureDialog.value = null
  scheduleSave()
}

// ===== v0.7.100 — Sort handlers =====
const openSortModal = () => {
  sortColInput.value = 'A'
  sortDirectionInput.value = 'asc'
  sortStartRowInput.value = 1
  sortEndRowInput.value = rows.value.length
  featureDialog.value = 'sort'
}
const onSortCommit = () => {
  const ci = colToIndex(sortColInput.value.trim())
  if (ci < 0) {
    MessagePlugin.error('列格式不正确,例如 A/B/C/...')
    return
  }
  const start = Math.max(0, (sortStartRowInput.value || 1) - 1)
  const end = Math.min(rows.value.length - 1, (sortEndRowInput.value || rows.value.length) - 1)
  if (end <= start) {
    MessagePlugin.error('结束行必须大于起始行')
    return
  }
  const dir = sortDirectionInput.value === 'desc' ? -1 : 1
  const slice = rows.value.slice(start, end + 1)
  const sorted = slice.slice().sort((a, b) => {
    const av = a[ci] ?? ''
    const bv = b[ci] ?? ''
    const an = Number(av)
    const bn = Number(bv)
    if (!Number.isNaN(an) && !Number.isNaN(bn) && av !== '' && bv !== '') {
      return dir * (an - bn)
    }
    return dir * String(av).localeCompare(String(bv))
  })
  const newRows = rows.value.slice()
  for (let i = 0; i < sorted.length; i++) newRows[start + i] = sorted[i]
  // Sync only the selected row range. Rows outside the sort range must keep
  // their existing Yjs data and collaborator edits.
  const cellMap = getSheetCellMap(activeSheetName.value)
  if (cellMap && handle) {
    handle!.ydoc.transact(() => {
      for (let ri = start; ri <= end; ri++) cellMap.delete(String(ri))
      for (let i = 0; i < sorted.length; i++) {
        const newKey = String(start + i)
        const yrow = new Y.Map<string>()
        const srcRow = sorted[i]
        for (let c = 0; c < srcRow.length; c++) {
          const v = srcRow[c]
          if (v !== '' && v !== null && v !== undefined) yrow.set(String(c), String(v))
        }
        cellMap.set(newKey, yrow)
      }
    })
  }
  rows.value = newRows
  MessagePlugin.success(`已按 ${sortColInput.value.toUpperCase()} 列${sortDirectionInput.value === 'desc' ? '降序' : '升序'}排列 ${sorted.length} 行`)
  featureDialog.value = null
  scheduleSave()
}

// ===== v0.7.102 — Defined Names (workbook-level named ranges) =====
/** Snapshot the Yjs defined-names array back into a plain JS list (for wb save). */
// v0.7.105 — apply a single sheet rename to a defined-name formula. Uses
// the same FORMULA_REFERENCE_PATTERN + qualifierMatches as xlsxSheets.ts so
// it stays aligned with the rename path that mutates <definedName> in
// workbook.xml for non-managed entries.
const applyRenameToDefinedFormula = (formula: string, oldName: string, newName: string): string => {
  // Excel formulas use single quotes for sheet names with special chars;
  // plain ASCII names like "Sheet1" stay unquoted. We rewrite both styles.
  const escape = (s: string) => s.replaceAll("'", "''")
  const newQuoted = `'${escape(newName)}'`
  // Replace qualified refs first (quoted, then unquoted). Avoid touching
  // string literals — split on `"` and skip odd-indexed segments.
  return formula
    .split('"')
    .map((segment, idx) => {
      if (idx % 2 === 1) return segment
      // 1) quoted qualifier: 'Sheet1'! -> 'Sales'!
      const quotedOld = `'${escape(oldName)}'!`
      const quotedNew = `${newQuoted}!`
      let out = segment.split(quotedOld).join(quotedNew)
      // 2) unquoted qualifier: only when oldName / newName are simple identifiers
      //    and not a substring of another name. Excel's grammar allows
      //    [A-Za-z_][A-Za-z0-9_.]* before the "!".
      if (/^[A-Za-z_][A-Za-z0-9_.]*$/.test(oldName) && /^[A-Za-z_][A-Za-z0-9_.]*$/.test(newName)) {
        const re = new RegExp('(^|[^A-Za-z0-9_.])' + oldName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '!', 'g')
        out = out.replace(re, (m, lead) => `${lead}${newName}!`)
      }
      return out
    })
    .join('"')
}

const readDefinedNamesFromY = (arr: Y.Array<Y.Map<unknown>>): DefinedNameEntry[] => {
  const out: DefinedNameEntry[] = []
  arr.toArray().forEach((m) => {
    const name = String(m.get('name') ?? '').trim()
    const formula = String(m.get('formula') ?? '').trim()
    if (!name || !formula) return
    const sheetRaw = m.get('sheetIndex')
    const sheetIndex = typeof sheetRaw === 'number' && sheetRaw >= 0 ? sheetRaw : undefined
    out.push({ name, formula, sheetIndex })
  })
  return out
}
/** Reactive list of defined names, derived from Yjs (live across collaborators). */
/** v0.7.102 — reactive list of defined names, refreshed by the Yjs observer
 *  (a plain computed won't track Y.Array mutations, so we mirror into a ref). */
const definedNamesList = ref<DefinedNameEntry[]>([])
const openNamesModal = () => {
  newDefinedName.value = ''
  newDefinedFormula.value = ''
  newDefinedScope.value = -1
  featureDialog.value = 'names'
}
const addDefinedName = () => {
  if (!ydefinedNames || !handle) return
  const name = newDefinedName.value.trim()
  const formula = newDefinedFormula.value.trim()
  if (!name || !formula) return
  // Excel name rules: letters / _ / \ start, then letters / digits / _ / . / \.
  // Reject if it looks like a cell reference or a reserved literal.
  if (!/^[\p{L}_\\][\p{L}\p{N}_.\\]*$/u.test(name) ||
      /^(?:[A-Za-z]{1,3}[0-9]+|[Rr][0-9]*[Cc][0-9]*)$/.test(name) ||
      name.toLowerCase() === 'true' || name.toLowerCase() === 'false' ||
      name.startsWith('_xlnm')) {
    MessagePlugin.error(`"${name}" 不是合法的命名区域名称`)
    return
  }
  const scope = newDefinedScope.value
  handle.ydoc.transact(() => {
    const m = new Y.Map<unknown>()
    m.set('name', name)
    m.set('formula', formula)
    if (scope >= 0) m.set('sheetIndex', scope)
    ydefinedNames!.push([m])
  })
  newDefinedName.value = ''
  newDefinedFormula.value = ''
  scheduleSave()
}
const deleteDefinedName = (idx: number) => {
  if (!ydefinedNames) return
  if (idx < 0 || idx >= ydefinedNames.length) return
  ydefinedNames.doc?.transact(() => {
    ydefinedNames!.delete(idx, 1)
  })
  scheduleSave()
}

// v0.7.105 — Create-from-selection. Pre-fills the formula from selectedRangeA1
// and lets the user edit the name + scope before committing.
const addDefinedNameFromSelection = () => {
  if (!selectedRangeA1.value) return
  // Default name: e.g. "Range_A1_D5" — editable before commit.
  const r = selectedRange.value!
  const suggested = `Range_${cellLabel(r.r1, r.c1)}_${cellLabel(r.r2, r.c2)}`
  newDefinedName.value = suggested
  newDefinedFormula.value = selectedRangeA1.value
  // Default scope = workbook (Excel semantics for a fully-qualified Sheet!A1:B2 formula).
  newDefinedScope.value = -1
}

// ===== v0.7.99 — Find & Replace handlers =====
const openFindModal = () => {
  findSearchInput.value = ''
  findReplaceInput.value = ''
  findMatchCaseInput.value = false
  featureDialog.value = 'find'
}
interface FindMatch {
  row: number
  column: number
  before: string
}
const findMatches = computed<FindMatch[]>(() => {
  const needle = findSearchInput.value
  if (!needle) return []
  const out: FindMatch[] = []
  for (let r = 0; r < rows.value.length; r++) {
    const row = rows.value[r] || []
    for (let c = 0; c < row.length; c++) {
      const v = row[c] ?? ''
      const hay = findMatchCaseInput.value ? v : v.toLowerCase()
      const ndl = findMatchCaseInput.value ? needle : needle.toLowerCase()
      if (hay.includes(ndl)) {
        const before = v.length > 40 ? v.slice(0, 40) + '…' : v
        out.push({ row: r, column: c, before })
      }
    }
  }
  return out
})
const onFindCommit = () => {
  if (!findSearchInput.value || !findMatches.value.length) return
  const repl = findReplaceInput.value
  const m = findMatchCaseInput.value
  const re = m
    ? null
    : new RegExp(findSearchInput.value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi')
  let count = 0
  for (const hit of findMatches.value) {
    const v = rows.value[hit.row]?.[hit.column] ?? ''
    const next = m ? v.replaceAll(findSearchInput.value, repl) : v.replace(re!, repl)
    if (next !== v) {
      setCell(hit.row, hit.column, next)
      count += 1
    }
  }
  MessagePlugin.success(`已替换 ${count} 个单元格`)
  findSearchInput.value = ''
  findReplaceInput.value = ''
}
const onFindClear = () => {
  findSearchInput.value = ''
  findReplaceInput.value = ''
}

// ===== v0.7.45 — Hyperlink modal handlers =====
const openHyperlinkModal = () => {
  linkRowInput.value = ''
  linkColInput.value = ''
  linkTargetInput.value = ''
  featureDialog.value = 'hyperlink'
}
const onHyperlinkCommit = () => {
  const row = Number(linkRowInput.value) - 1
  const col = colToIndex(linkColInput.value.trim())
  const target = linkTargetInput.value.trim()
  if (Number.isNaN(row) || row < 0 || col < 0 || !target) {
    MessagePlugin.error('请填写完整:行/列/目标')
    return
  }
  const existing = hyperlinksBySheet.value[activeSheet.value] || []
  hyperlinksBySheet.value[activeSheet.value] = existing.concat([{ row, column: col, target }])
  linkRowInput.value = ''
  linkColInput.value = ''
  linkTargetInput.value = ''
  scheduleSave()
}
const onHyperlinkClear = () => {
  hyperlinksBySheet.value[activeSheet.value] = []
  featureDialog.value = null
  scheduleSave()
}

// ===== v0.7.45 — Table modal handlers =====
const openTableModal = () => {
  tableNameInput.value = ''
  tableStartRowInput.value = '1'
  tableStartColInput.value = 'A'
  tableEndRowInput.value = '3'
  tableEndColInput.value = 'C'
  tableColumnsInput.value = 'Q1, Q2, Q3'
  featureDialog.value = 'table'
}
const onTableCommit = () => {
  const name = (tableNameInput.value || '').trim()
  const startRow = Number(tableStartRowInput.value) - 1
  const startCol = colToIndex(tableStartColInput.value.trim())
  const endRow = Number(tableEndRowInput.value) - 1
  const endCol = colToIndex(tableEndColInput.value.trim())
  const columnNames = tableColumnsInput.value.split(',').map((s) => s.trim()).filter(Boolean)
  if (!name || Number.isNaN(startRow) || Number.isNaN(endRow) || startCol < 0 || endCol < 0
      || endRow <= startRow || endCol < startCol || !columnNames.length
      || (endCol - startCol + 1) !== columnNames.length) {
    MessagePlugin.error('表格参数不合法,请检查行/列范围和列名数量')
    return
  }
  if (!/^[^\\/?*\[\]:]{1,255}$/.test(name)) {
    MessagePlugin.error('表名含非法字符或过长')
    return
  }
  const existing = tablesBySheet.value[activeSheet.value] || []
  if (existing.some((t) => t.name.toLowerCase() === name.toLowerCase())) {
    MessagePlugin.error(`表名 "${name}" 已存在`)
    return
  }
  tablesBySheet.value[activeSheet.value] = existing.concat([{
    worksheetPath: '',  // resolved at save time
    area: { startRow, startColumn: startCol, endRow, endColumn: endCol },
    name,
    columnNames,
    bandedRows: true,
  }])
  tableNameInput.value = ''
  scheduleSave()
}
const onTableClear = () => {
  tablesBySheet.value[activeSheet.value] = []
  featureDialog.value = null
  scheduleSave()
}

// Helper: convert column letter(s) to 0-based index (A=0, Z=25, AA=26)
const colToIndex = (s: string): number => {
  let n = 0
  for (const ch of s.toUpperCase()) {
    if (ch < 'A' || ch > 'Z') return -1
    n = n * 26 + (ch.charCodeAt(0) - 64)
  }
  return n - 1
}

// ===== v0.7.44 — Sheet Manage modal handlers =====
const openSheetManageModal = () => {
  sheetRenameDraftBySheet.value = {}
  for (let si = 0; si < sheets.value.length; si++) {
    sheetRenameDraftBySheet.value[si] = sheets.value[si]?.name || ''
  }
  featureDialog.value = 'sheetManage'
}

const moveSheetUp = (si: number) => {
  if (si <= 0) return
  // Persist current edits first
  sheets.value[activeSheet.value] = {
    name: sheets.value[activeSheet.value]?.name || ('Sheet' + (activeSheet.value + 1)),
    rows: rows.value.map((r) => r.slice()),
  }
  const cur = activeSheet.value
  const a = sheets.value[si - 1]
  const b = sheets.value[si]
  if (!a || !b) return
  sheets.value[si - 1] = b
  sheets.value[si] = a
  if (cur === si - 1) activeSheet.value = si
  else if (cur === si) activeSheet.value = si - 1
  rows.value = sheets.value[activeSheet.value].rows.map((r) => r.slice())
  scheduleSave()
}

const moveSheetDown = (si: number) => {
  if (si >= sheets.value.length - 1) return
  // Persist current edits first
  sheets.value[activeSheet.value] = {
    name: sheets.value[activeSheet.value]?.name || ('Sheet' + (activeSheet.value + 1)),
    rows: rows.value.map((r) => r.slice()),
  }
  const cur = activeSheet.value
  const a = sheets.value[si]
  const b = sheets.value[si + 1]
  if (!a || !b) return
  sheets.value[si] = b
  sheets.value[si + 1] = a
  if (cur === si) activeSheet.value = si + 1
  else if (cur === si + 1) activeSheet.value = si
  rows.value = sheets.value[activeSheet.value].rows.map((r) => r.slice())
  scheduleSave()
}

const toggleSheetHidden = (si: number) => {
  sheetHiddenBySheet.value[si] = !sheetHiddenBySheet.value[si]
  scheduleSave()
}

const removeSheet = (si: number) => {
  if (sheets.value.length <= 1) return
  const removedName = sheets.value[si] && sheets.value[si].name
  sheets.value.splice(si, 1)
  // v0.7.93 -- broadcast sheet removal so collaborators drop the tab.
  if (ysheets && removedName && handle) {
    const yi = ysheets.toArray().indexOf(removedName)
    if (yi >= 0) {
      handle.ydoc.transact(() => {
        if (ysheets) ysheets.delete(yi, 1)
      })
    }
  }
  if (sheetCellRoot && removedName) sheetCellRoot.delete(removedName)
  commitSheetManifest()
  if (activeSheet.value >= sheets.value.length) activeSheet.value = sheets.value.length - 1
  rows.value = sheets.value[activeSheet.value].rows.map((r) => r.slice())
  // Cascade-clear per-sheet feature state for removed index.
  const restage = <T extends Record<number, any>>(rec: T) => {
    const out: Record<number, any> = {}
    for (const [k, v] of Object.entries(rec)) {
      const n = Number(k)
      if (n < si) out[n] = v
      else if (n > si) out[n - 1] = v
    }
    return out
  }
  freezeBySheet.value = restage(freezeBySheet.value)
  filterBySheet.value = restage(filterBySheet.value)
  cfBySheet.value = restage(cfBySheet.value)
  dvBySheet.value = restage(dvBySheet.value)
  sparkBySheet.value = restage(sparkBySheet.value)
  pageSetupBySheet.value = restage(pageSetupBySheet.value)
  sheetHiddenBySheet.value = restage(sheetHiddenBySheet.value)
  scheduleSave()
}

const applySheetRenames = () => {
  let changed = false
  for (const [k, v] of Object.entries(sheetRenameDraftBySheet.value)) {
    const si = Number(k)
    const oldName = sheets.value[si]?.name
    const newName = (v || '').trim()
    // eslint-disable-next-line no-console
    if (!newName || newName === oldName) continue
    const __passes = /^[^\/?*[\]:]{1,31}$/.test(newName)
    // eslint-disable-next-line no-console
    if (!__passes) {
      MessagePlugin.error(`工作表名 \"${newName}\" 含非法字符或过长(>31)`)
      return
    }
    sheets.value[si] = { ...sheets.value[si], name: newName }
    if (sheetCellRoot && oldName && oldName !== newName) {
      try {
        const oldMap = sheetCellRoot.get(oldName)
        if (oldMap) {
          // Y.Map can't be re-parented, so build a new map and clone entries
          // under the same transaction.
          const cloned = new Y.Map<Y.Map<string>>()
          oldMap.forEach((row, rowKey) => {
            const clonedRow = new Y.Map<string>()
            row.forEach((val, colKey) => clonedRow.set(colKey, val))
            cloned.set(rowKey, clonedRow)
          })
          sheetCellRoot.set(newName, cloned)
        }
        sheetCellRoot.delete(oldName)
      } catch (e) {
        // eslint-disable-next-line no-console
        console.warn('[v0.7.105] sheetCellRoot rename failed:', e)
      }
    }
    changed = true
  }
  if (changed) {
    commitSheetManifest()
    scheduleSave()
  }
  featureDialog.value = null
}

const rows = ref<string[][]>(
  Array.from({ length: 5 }, () => Array.from({ length: cols.value.length }, () => '')),
)
const loading = ref(false)
const downloading = ref(false)
const uploading = ref(false)
const saveLabel = ref('未修改')
const saveError = ref<string | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

let handle: ReturnType<typeof useYjsCollabDoc> | null = null

const remoteSelections = ref<Array<{
  clientId: number
  displayName: string
  color: string
  cell?: { ri: number; ci: number } | null
}>>([])

let ymap: Y.Map<Y.Map<string>> | null = null
let ycols: Y.Array<string> | null = null
type SheetCellMap = Y.Map<Y.Map<string>>
let sheetCellRoot: Y.Map<SheetCellMap> | null = null
// v0.7.93 -- sheet-name list as a Y.Array so add/remove propagates to
// all collaborators in real time. Cells stay in the shared
// (sheet:cells) ymap; per-sheet cell namespace is the next milestone.
let ysheets: Y.Array<string> | null = null
let ysheetSnapshot: Y.Map<string> | null = null
// v0.7.102 — workbook-level named ranges (Excel Defined Names), synced via Yjs
let ydefinedNames: Y.Array<Y.Map<unknown>> | null = null
let wb: XlsxAdapterWorkbook = newXlsxWorkbook()
const __seedWb = () => { lastSavedSheetNames.value = wb.sheets.map((s) => s.name) }
__seedWb()
let saveTimer: ReturnType<typeof setTimeout> | null = null

const getSheetCellMap = (sheetName: string): SheetCellMap | null => {
  if (!sheetCellRoot) return null
  let cellMap = sheetCellRoot.get(sheetName)
  if (!cellMap) {
    cellMap = new Y.Map<Y.Map<string>>()
    sheetCellRoot.set(sheetName, cellMap)
  }
  return cellMap
}

const connectionLabel = computed(() => {
  if (saveError.value) return `保存错误：${saveError.value}`
  if (connected.value) return '已连接'
  return '连接中...'
})

const kindLabel = computed(() => 'Excel 表格 (.xlsx)')

const savetagClass = computed(() => ({
  dirty: saveLabel.value === '有未保存的修改',
  saving: saveLabel.value === '保存中...',
}))

const initialOf = (name: string) => (name || '?').trim().slice(0, 1).toUpperCase()

const downloadAsUint8Array = async (): Promise<Uint8Array> => {
  const blob = await downloadCollabDocBytes(props.docId)
  const buf = await blob.arrayBuffer()
  return new Uint8Array(buf)
}

const setup = async () => {
  if (!props.docId || !props.token) return
  handle = useYjsCollabDoc({ docId: props.docId, token: props.token, displayName: props.displayName })
  // v0.7.38 — remote cell-selection awareness
  if (handle.remoteSelections) {
    remoteSelections.value = handle.remoteSelections.value as any
    watch(handle.remoteSelections, (v) => (remoteSelections.value = (v ?? []) as any))
  }
  connected.value = Boolean(handle.connected.value)
  peers.value = (handle.peers.value ?? []) as Array<{ clientId: number; displayName: string; color: string }>
  error.value = (handle.error.value ?? null) as string | null
  watch(handle.connected, (v) => (connected.value = Boolean(v)))
  watch(handle.peers, (v) => (peers.value = (v ?? []) as Array<{ clientId: number; displayName: string; color: string }>))
  watch(handle.error, (v) => (error.value = (v ?? null) as string | null))

  ycols = handle.ydoc.getArray<string>('sheet:cols')
  ymap = handle.ydoc.getMap<Y.Map<string>>('sheet:cells')
  sheetCellRoot = handle.ydoc.getMap<SheetCellMap>('sheet:cells:by-sheet')
  ysheets = handle.ydoc.getArray<string>('sheet:names')
  ysheetSnapshot = handle.ydoc.getMap<string>('sheet:names:manifest')
  ydefinedNames = handle.ydoc.getArray<Y.Map<unknown>>('sheet:definedNames')

  loading.value = true
  try {
    let bytes: Uint8Array | null = null
    try {
      bytes = await downloadAsUint8Array()
    } catch (e) {
      bytes = null
    }
    if (bytes) {
      wb = await openXlsx(bytes)
      sheets.value = wb.sheets.map((sh) => ({ name: sh.name, rows: sh.rows.map((r) => r.map((c) => String(c.v ?? ''))) }))
      // v0.7.105 — baseline: these are the names on disk; renames are diffed
      // against this snapshot on every flushSave.
      lastSavedSheetNames.value = sheets.value.map((s) => s.name)
      activeSheet.value = 0
      const first = sheets.value[0]
      if (first) {
        const maxCol = first.rows.reduce((m, r) => Math.max(m, r.length), 0)
        cols.value = Array.from({ length: Math.max(maxCol, 1) }, (_, i) => colName(i))
        rows.value = first.rows.map((r) => {
          const padded = r.slice()
          while (padded.length < cols.value.length) padded.push('')
          return padded
        })
        if (rows.value.length === 0) {
          rows.value = [Array.from({ length: cols.value.length }, () => '')]
        }
      } else {
        sheets.value = [{ name: 'Sheet1', rows: [Array.from({ length: cols.value.length }, () => '')] }]
      }
    }
    if (sheetCellRoot && handle) {
      handle.ydoc.transact(() => {
        for (const sheet of sheets.value) {
          const cellMap = getSheetCellMap(sheet.name)
          if (!cellMap || cellMap.size > 0) continue
          for (let ri = 0; ri < sheet.rows.length; ri++) {
            const row = sheet.rows[ri] || []
            for (let ci = 0; ci < row.length; ci++) {
              if (row[ci] === '') continue
              let yrow = cellMap.get(String(ri))
              if (!yrow) {
                yrow = new Y.Map<string>()
                cellMap.set(String(ri), yrow)
              }
              yrow.set(String(ci), row[ci])
            }
          }
        }
      })
    }
    if (ymap && ymap.size > 0 && sheets.value[0] && handle) {
      const target = getSheetCellMap(sheets.value[0].name)
      if (target && target.size === 0) {
        handle.ydoc.transact(() => {
          ymap!.forEach((yrow, rowKey) => {
            target.set(rowKey, yrow)
          })
          ymap!.clear()
        })
      }
    }
    // v0.7.102 — seed Yjs defined names from the workbook's parsed names on first load.
    if (ydefinedNames && ydefinedNames.length === 0 && wb.definedNames.length > 0 && handle) {
      handle.ydoc.transact(() => {
        for (const entry of wb.definedNames) {
          const m = new Y.Map<unknown>()
          m.set('name', entry.name)
          m.set('formula', entry.formula)
          if (entry.sheetIndex !== undefined) m.set('sheetIndex', entry.sheetIndex)
          ydefinedNames!.push([m])
        }
      })
    }
    // Observe remote defined-name changes and persist into wb so saves round-trip.
    if (ydefinedNames) {
      ydefinedNames.observeDeep(() => {
        wb.definedNames = readDefinedNamesFromY(ydefinedNames!)
        definedNamesList.value = wb.definedNames
      })
      // Seed the ref from the current Yjs state so the modal renders immediately.
      definedNamesList.value = readDefinedNamesFromY(ydefinedNames)
    }
  const applyRemoteSheetNames = () => {
    const manifest = ysheetSnapshot?.get('names')
    if (manifest) {
      try {
        const names = JSON.parse(manifest)
        if (Array.isArray(names) && names.every((name) => typeof name === 'string')) {
          const current = sheets.value.map((sheet) => sheet.name)
          if (JSON.stringify(current) !== JSON.stringify(names)) {
            sheets.value = names.map((name, index) => {
              const existing = sheets.value[index]
              return existing && existing.name === name
                ? existing
                : { name, rows: [Array.from({ length: cols.value.length }, () => '')] }
            })
            if (activeSheet.value >= sheets.value.length) activeSheet.value = Math.max(0, sheets.value.length - 1)
            rows.value = sheets.value[activeSheet.value]?.rows?.map((row) => row.slice()) || [[]]
          }
          return
        }
      } catch {
        // Fall through to the legacy Y.Array delta path.
      }
    }
    if (!ysheets) return
    const remote = ysheets.toArray()
    if (remote.length === 0) return
    const current = sheets.value.map((sheet) => sheet.name)
    if (JSON.stringify(remote) === JSON.stringify(current)) return
    const currentSet = new Set(current)
    const extra = remote.filter((name) => !currentSet.has(name))
    if (current.length > 0 && remote.every((name) => currentSet.has(name)) && remote.length <= current.length) return
    const names = extra.length > 0 ? [...current, ...extra] : remote
    if (JSON.stringify(names) === JSON.stringify(current)) return
    sheets.value = names.map((name, index) => {
      const existing = sheets.value[index]
      return existing && existing.name === name
        ? existing
        : { name, rows: [Array.from({ length: cols.value.length }, () => '')] }
    })
    if (activeSheet.value >= sheets.value.length) activeSheet.value = Math.max(0, sheets.value.length - 1)
    rows.value = sheets.value[activeSheet.value]?.rows?.map((row) => row.slice()) || [[]]
  }
  ysheets?.observe(applyRemoteSheetNames)
  ysheetSnapshot?.observe(applyRemoteSheetNames)
  applyRemoteSheetNames()
  } catch (e: any) {
    error.value = `加载失败：${e?.message || e}`
  } finally {
    loading.value = false
  }

  // Sync from Yjs
  const remoteCols = ycols.toArray()
  if (remoteCols.length > 0) cols.value = remoteCols
  handle.ydoc.transact(() => {
    if (ycols && ycols.length === 0) ycols.insert(0, cols.value)
  })
  ycols.observe(() => {
    if (!ycols) return
    const next = ycols.toArray()
    if (JSON.stringify(next) !== JSON.stringify(cols.value)) {
      cols.value = next
      rows.value = rows.value.map((r) => {
        if (r.length < next.length) return [...r, ...Array.from({ length: next.length - r.length }, () => '')]
        if (r.length > next.length) return r.slice(0, next.length)
        return r
      })
    }
  })
  sheetCellRoot?.observeDeep(syncFromY)
  syncFromY()
}

const colName = (i: number): string => {
  let n = i
  let s = ''
  while (n >= 0) {
    s = String.fromCharCode(65 + (n % 26)) + s
    n = Math.floor(n / 26) - 1
  }
  return s
}

const syncFromY = () => {
  const cellMap = getSheetCellMap(activeSheetName.value)
  if (!cellMap) return
  const next: string[][] = []
  for (let i = 0; i < rows.value.length; i++) {
    const rowKey = String(i)
    const yrow = cellMap.get(rowKey)
    const row: string[] = []
    for (let ci = 0; ci < cols.value.length; ci++) {
      row.push(yrow ? yrow.get(String(ci)) || '' : '')
    }
    next.push(row)
  }
  rows.value = next
}

const cellText = (ri: number, ci: number) => rows.value[ri]?.[ci] ?? ''

/** A cell may be a raw string (from Yjs) or an XlsxAdapterCell (after
 *  openXlsx). Normalize to one shape so the helpers below stay symmetric. */
const normalizeCell = (raw: unknown): XlsxAdapterCell => {
  if (raw == null || raw === '') return { v: '' }
  if (typeof raw === 'object') return raw as XlsxAdapterCell
  return { v: String(raw) }
}
const cellFormula = (ri: number, ci: number) => {
  const c = normalizeCell(rows.value[ri]?.[ci])
  if (c.f) return c.f
  const v = c.v
  return typeof v === 'string' && v.startsWith('=') ? v.slice(1) : ''
}
const cellPercent = (ri: number, ci: number) => {
  const c = normalizeCell(rows.value[ri]?.[ci])
  return (c.z ?? '').includes('%')
}

/** Build an XlsxAdapterCell from a raw string cell. Strings starting with
 *  '=' become formulas (cell.f = the right-hand side); pure numerics become
 *  number cells with no format. Anything else becomes a text cell. */
const buildCell = (raw: string | undefined): XlsxAdapterCell => {
  const v = raw ?? ''
  if (typeof v === 'string' && v.startsWith('=') && v.length > 1) {
    return { v: '', f: v.slice(1) }
  }
  if (typeof v === 'string' && /^-?\d+(\.\d+)?$/.test(v)) {
    return { v: Number(v) }
  }
  return { v }
}

const setCell = (ri: number, ci: number, value: string) => {
  // v0.7.77 — soft optimistic lock: a peer currently editing this cell blocks our write.
  const check = checkEditAllowed(remoteSelections.value, myClientId.value, ri, ci)
  if (!check.allowed) {
    // eslint-disable-next-line no-console
    console.warn(`[CollabSheetEditor] cell (${ri},${ci}) is being edited by ${check.locker}; local edit rejected`)
    // Restore the input value to the cell's current contents so the user sees why nothing happened.
    const cellInput = document.querySelector<HTMLInputElement>(`input[data-cell="${ri}-${ci}"]`)
    if (cellInput) cellInput.value = rows.value[ri]?.[ci] ?? ''
    return
  }
  rows.value[ri][ci] = value
  const cellMap = getSheetCellMap(activeSheetName.value)
  if (!cellMap || !handle) return
  handle!.ydoc.transact(() => {
    let yrow = cellMap.get(String(ri))
    if (!yrow) {
      yrow = new Y.Map<string>()
      cellMap.set(String(ri), yrow)
    }
    yrow.set(String(ci), value)
  })
  scheduleSave()
}

const addColumn = () => {
  if (!ycols || !handle) return
  handle!.ydoc.transact(() => ycols!.push([`列${cols.value.length + 1}`]))
  rows.value = rows.value.map((r) => [...r, ''])
  scheduleSave()
}

const addRow = () => {
  rows.value.push(Array.from({ length: cols.value.length }, () => ''))
  syncFromY()
  scheduleSave()
}

const delLastColumn = () => {
  if (cols.value.length <= 1) return
  if (!ycols || !handle) return
  handle!.ydoc.transact(() => {
    const localCols = ycols!
    localCols.delete(localCols.length - 1, 1)
  })
  rows.value = rows.value.map((r) => r.slice(0, -1))
  const cellMap = getSheetCellMap(activeSheetName.value)
  if (cellMap) {
    handle!.ydoc.transact(() => {
      cellMap.forEach((yrow, key) => {
        const lastKey = String(cols.value.length)
        yrow.delete(lastKey)
        if (yrow.size === 0) cellMap.delete(key)
      })
    })
  }
  scheduleSave()
}

const delLastRow = () => {
  if (rows.value.length <= 1) return
  rows.value = rows.value.slice(0, -1)
  getSheetCellMap(activeSheetName.value)?.delete(String(rows.value.length))
  scheduleSave()
}

const renameColumn = (ci: number, name: string) => {
  if (!ycols || !handle) return
  handle!.ydoc.transact(() => ycols!.delete(ci, 1))
  handle!.ydoc.transact(() => ycols!.insert(ci, [name]))
  cols.value[ci] = name
  scheduleSave()
}

const scheduleSave = () => {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(() => flushSave(), 1500)
}

// Build the per-sheet XML transform pipeline. Each sheet with feature
// state produces a (worksheetXml) => worksheetXml function that
// sequentially applies the relevant adapters. Sheets with no feature
// state get a no-op transform (never enters the pipeline).
interface SheetPathResolver {
  /** Resolve the worksheet XML path inside the package for a given sheet name. */
  resolveWorksheetPath(name: string): Promise<string | null>
}

const buildFeaturePipeline = (): {
  transforms: Record<string, (xml: string) => string>
  packageTransformer: ((pkg: MutablePackage, paths: SheetPathResolver) => Promise<void>) | null
} => {
  const transforms: Record<string, (xml: string) => string> = {}
  let hasMultiFile = false
  sheets.value.forEach((sh, idx) => {
    const fr = freezeBySheet.value[idx]
    const fs = filterBySheet.value[idx]
    const cf = cfBySheet.value[idx]
    const dv = dvBySheet.value[idx]
    const sp = sparkBySheet.value[idx]
    const ps = pageSetupBySheet.value[idx]
    const nt = notesBySheet.value[idx]
    const tb = tablesBySheet.value[idx]
    const hasSingle = fr || fs || cf?.length || dv?.length || sp?.length || ps
    const hasMulti = nt?.length || tb?.length
    if (hasMulti) hasMultiFile = true
    if (!hasSingle && !hasMulti) return
    transforms[sh.name] = (xml: string) => {
      let next = xml
      if (fr) next = injectSheetViewFreeze(next, fr)
      if (fs) next = applyFilterState(next, fs)
      if (cf?.length) next = applyCfRules(next, cf, { internDxf: () => 0 })
      if (dv?.length) next = applyDvRules(next, dv)
      if (sp && sp.length) next = applySparklineAdditions(next, sp as readonly SparklineGroupAdd[])
      if (ps) next = applyPageSetupState(next, ps)
      return next
    }
  })
  if (!hasMultiFile) return { transforms, packageTransformer: null }

  const packageTransformer = async (pkg: MutablePackage, paths: SheetPathResolver): Promise<void> => {
    const touched = new Set<string>()
    for (let idx = 0; idx < sheets.value.length; idx++) {
      const sh = sheets.value[idx]
      const nt = notesBySheet.value[idx]
      const tb = tablesBySheet.value[idx]
      const wsPath = await paths.resolveWorksheetPath(sh.name)
      if (!wsPath) continue
      if (nt && nt.length) {
        await applySheetNotes(pkg as any, wsPath, nt, touched)
      }
      if (tb && tb.length) {
        // Each table addition needs worksheetPath set to the resolved path.
        const resolved: TableAddition[] = tb.map((t) => ({
          ...t,
          worksheetPath: wsPath,
        }))
        await applyTableAdditions(pkg as any, resolved, touched)
      }
    }
    // Hyperlinks — must run LAST because it patches worksheet.xml + rels.xml.
    for (let idx = 0; idx < sheets.value.length; idx++) {
      const sh = sheets.value[idx]
      const hl = hyperlinksBySheet.value[idx]
      if (!hl?.length) continue
      const wsPath = await paths.resolveWorksheetPath(sh.name)
      if (!wsPath) continue
      const wsXml = await pkg.readText(wsPath)
      const relsPath = wsPath.replace(/^xl\/worksheets\//, 'xl/worksheets/_rels/').replace(/\.xml$/, '.xml.rels')
      let relsXml: string | null = null
      if (await pkg.has(relsPath)) relsXml = await pkg.readText(relsPath)
      const patch = applyHyperlinkEdits(wsXml, relsXml, hl)
      pkg.write(wsPath, patch.worksheetXml)
      if (patch.relsChanged && patch.relsXml !== null) {
        pkg.write(relsPath, patch.relsXml)
      }
    }
  }
  return { transforms, packageTransformer }
}

// Tiny inline freeze-pane injector — delegates to xlsxAdapter via the
// existing readXlsxFreeze + injectSheetView helpers when present, else
// produces a minimal <sheetView>/<pane> element.
const injectSheetViewFreeze = (xml: string, pane: { rows: number; cols: number }): string => {
  const ySplit = pane.rows
  const xSplit = pane.cols
  if (ySplit <= 0 && xSplit <= 0) return xml
  const topLeftCell = `${colName(xSplit)}${ySplit + 1}`
  const state = ySplit > 0 && xSplit > 0 ? 'frozen' : 'frozenSplit'
  const paneXml =
    `<pane xSplit="${xSplit}" ySplit="${ySplit}" topLeftCell="${topLeftCell}" activePane="bottomRight" state="${state}"/>`
  const sheetViewXml = `<sheetView><selection pane="bottomRight" activeCell="${topLeftCell}" sqref="${topLeftCell}"/></sheetView>`.replace(
    '<sheetView>',
    `<sheetView workbookViewId="0">`,
  )
  // Strip any pre-existing sheetView/pane so re-applying is idempotent.
  const stripped = xml
    .replace(/<sheetView[^>]*>[\s\S]*?<\/sheetView>/g, '')
    .replace(/<pane[^/>]*\/?>/g, '')
  const fragment = sheetViewXml.replace('<selection', paneXml + '<selection')
  // sheetView must come before sheetData / pageMargins; insert just after <worksheet ...>.
  return stripped.replace(/(<worksheet[^>]*>)/, `$1${fragment}`)
}

const flushSave = async () => {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
  saveLabel.value = '保存中...'
  try {
    sheets.value[activeSheet.value] = {
      name: sheets.value[activeSheet.value]?.name || ('Sheet' + (activeSheet.value + 1)),
      rows: rows.value.map((r) => r.slice()),
    }
    // v0.7.105 — diff current sheet names against the last saved names so
    // applyDefinedNamesToBytes can rewrite <definedName> formulas that
    // reference renamed ones. Tracked on a ref so we don't compare against
    // a stale wb.sheets snapshot (which is mutated each save).
    const currentNames = sheets.value.map((s) => s.name)
    const renamedSheets: Array<{ old: string; new: string }> = []
    if (lastSavedSheetNames.value.length === currentNames.length) {
      for (let i = 0; i < currentNames.length; i++) {
        const old = lastSavedSheetNames.value[i]
        const next = currentNames[i]
        if (old && next && old !== next) renamedSheets.push({ old, new: next })
      }
    } else {
      // length mismatch (add/remove) — best-effort: pair by stable index
      // for the overlapping range.
      const overlap = Math.min(lastSavedSheetNames.value.length, currentNames.length)
      for (let i = 0; i < overlap; i++) {
        const old = lastSavedSheetNames.value[i]
        const next = currentNames[i]
        if (old && next && old !== next) renamedSheets.push({ old, new: next })
      }
    }
    // eslint-disable-next-line no-console
    wb.sheets = sheets.value.map((sh) => ({
      name: sh.name,
      rows: sh.rows.map((r) => r.map((v) => buildCell(v))),
    }))
    if (renamedSheets.length) wb.renamedSheets = renamedSheets
    // v0.7.105 — apply the rename map to wb.definedNames formulas so the
    // saved workbook's <definedName> references follow the new sheet names.
    // The workbook.xml pass below applies the same map to any preserved
    // entries (built-in / hidden), so both paths converge.
    // v0.7.105 — dedupe wb.definedNames by name+sheetIndex BEFORE applying
    // renames. Older yjs observers can re-broadcast a defined name after a
    // reload, leaving two entries with the same name in the wb snapshot;
    // applyDefinedNamesState appends them verbatim and Excel rejects the
    // workbook with "name defined twice". The latest entry wins.
    if (wb.definedNames?.length) {
      const seen = new Map<string, DefinedNameEntry>()
      for (const e of wb.definedNames) {
        const key = `${e.sheetIndex ?? -1}\${e.name}`
        if (!seen.has(key)) seen.set(key, e)
        // newer overrides older; the wb snapshot is appended in push order,
        // so iterating left→right keeps the last write.
        seen.set(key, e)
      }
      wb.definedNames = [...seen.values()]
    }
    if (renamedSheets.length && wb.definedNames?.length) {
      wb.definedNames = wb.definedNames.map((entry) => {
        // eslint-disable-next-line no-console
        let formula = entry.formula
        for (const { old: oldName, new: newName } of renamedSheets) {
          if (!oldName || !newName || oldName === newName) continue
          formula = applyRenameToDefinedFormula(formula, oldName, newName)
        }
        // eslint-disable-next-line no-console
        return { ...entry, formula }
      })
    }
    let bytes = await saveXlsxBytes(wb)
    const { transforms, packageTransformer } = buildFeaturePipeline()
    if (Object.keys(transforms).length > 0) {
      bytes = await transformWorkbook(bytes, transforms)
    }
    if (packageTransformer) {
      bytes = await transformPackage(bytes, async (pkg) => {
        await packageTransformer(pkg, {
          async resolveWorksheetPath(name) {
            const io = await inspectXlsx(bytes)
            return io.sheetPaths.get(name) ?? null
          },
        })
      })
    }
    // eslint-disable-next-line no-console
    await uploadCollabDocBytes(props.docId, bytes, `${props.title || 'collab-doc'}.xlsx`)
    // v0.7.105 — record the names that just landed on disk; clear rename map
    // so the next save treats these as the new baseline.
    lastSavedSheetNames.value = sheets.value.map((s) => s.name)
    wb.renamedSheets = []
    saveLabel.value = '已保存'
    saveError.value = null
    setTimeout(() => {
      if (saveLabel.value === '已保存') saveLabel.value = '未修改'
    }, 1500)
    // eslint-disable-next-line no-console
  } catch (e: any) {
    // eslint-disable-next-line no-console
    saveError.value = e?.message || String(e)
    saveLabel.value = '保存失败'
  }
}

const exportXlsx = async () => {
  downloading.value = true
  try {
    sheets.value[activeSheet.value] = {
      name: sheets.value[activeSheet.value]?.name || ('Sheet' + (activeSheet.value + 1)),
      rows: rows.value.map((r) => r.slice()),
    }
    wb.sheets = sheets.value.map((sh) => ({
      name: sh.name,
      rows: sh.rows.map((r) => r.map((v) => buildCell(v))),
    }))
    let bytes = await saveXlsxBytes(wb)
    const { transforms, packageTransformer } = buildFeaturePipeline()
    if (Object.keys(transforms).length > 0) {
      bytes = await transformWorkbook(bytes, transforms)
    }
    if (packageTransformer) {
      bytes = await transformPackage(bytes, async (pkg) => {
        await packageTransformer(pkg, {
          async resolveWorksheetPath(name) {
            const io = await inspectXlsx(bytes)
            return io.sheetPaths.get(name) ?? null
          },
        })
      })
    }
    const ab = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer
    const blob = new Blob([ab], {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${props.title || 'collab-doc'}.xlsx`
    a.click()
    URL.revokeObjectURL(a.href)
    MessagePlugin.success('已下载 .xlsx')
  } catch (e: any) {
    MessagePlugin.error(`下载失败：${e?.message || e}`)
  } finally {
    downloading.value = false
  }
}

const triggerUpload = () => fileInput.value?.click()

const onUploadFile = async (e: Event) => {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  uploading.value = true
  try {
    const buf = await file.arrayBuffer()
    const bytes = new Uint8Array(buf)
    wb = await openXlsx(bytes)
    const first = wb.sheets[0]
    if (first) {
      const maxCol = first.rows.reduce((m, r) => Math.max(m, r.length), 0)
      cols.value = Array.from({ length: maxCol }, (_, i) => colName(i))
      rows.value = first.rows.map((r) => {
        const padded = r.map((c) => String(c.v ?? ''))
        while (padded.length < maxCol) padded.push('')
        return padded
      })
      if (rows.value.length === 0) {
        rows.value = [Array.from({ length: cols.value.length }, () => '')]
      }
      // Reset only the active sheet's Yjs cell namespace.
      const cellMap = getSheetCellMap(activeSheetName.value)
      if (cellMap && handle) {
        handle.ydoc.transact(() => {
          for (const k of Array.from(cellMap.keys())) {
            cellMap.delete(k)
          }
          for (let i = 0; i < rows.value.length; i++) {
            const yrow = new Y.Map<string>()
            for (let ci = 0; ci < cols.value.length; ci++) {
              yrow.set(String(ci), rows.value[i][ci] || '')
            }
            cellMap.set(String(i), yrow)
          }
        })
      }
    }
    await uploadCollabDocBytes(props.docId, bytes, file.name)
    saveLabel.value = '已上传'
    MessagePlugin.success(`已上传 ${file.name}`)
  } catch (err: any) {
    MessagePlugin.error(`上传失败：${err?.message || err}`)
  } finally {
    uploading.value = false
    if (input) input.value = ''
  }
}

const teardown = () => {
  if (saveTimer) {
    clearTimeout(saveTimer)
    saveTimer = null
  }
  if (handle) {
    handle.destroy()
    handle = null
  }
  ymap = null
  ycols = null
  sheetCellRoot = null
}

setup()
watch(() => props.docId, () => { teardown(); setup() })
onBeforeUnmount(teardown)
</script>

<style scoped>
.collab-sheet-editor { display: flex; flex-direction: column; height: 100%; }
.collab-sheet-editor__toolbar {
  display: flex; align-items: center; gap: 8px; padding: 8px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  flex-wrap: wrap;
}
.collab-sheet-editor__title { font-weight: 600; }
.collab-sheet-editor__kind { font-size: 11px; padding: 2px 8px; border-radius: 999px; background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-sheet-editor__connection { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); }
.collab-sheet-editor__connection.connected { background: var(--td-success-color-1); color: var(--td-success-color-7); }
.collab-sheet-editor__savetag { font-size: 12px; padding: 2px 8px; border-radius: 999px; background: var(--td-bg-color-secondarycontainer); color: var(--td-text-color-secondary); }
.collab-sheet-editor__savetag.dirty { background: var(--td-warning-color-1); color: var(--td-warning-color-7); }
.collab-sheet-editor__savetag.saving { background: var(--td-brand-color-1); color: var(--td-brand-color-7); }
.collab-sheet-editor__add-col, .collab-sheet-editor__add-row, .collab-sheet-editor__export, .collab-sheet-editor__upload {
  padding: 4px 10px; font-size: 12px; border: 1px solid var(--td-component-stroke); border-radius: 4px; background: var(--td-bg-color-container); cursor: pointer;
}
.collab-sheet-editor__add-col:disabled, .collab-sheet-editor__add-row:disabled, .collab-sheet-editor__export:disabled, .collab-sheet-editor__upload:disabled { opacity: 0.5; cursor: not-allowed; }
.collab-sheet-editor__peers { display: flex; gap: 4px; margin-left: auto; }
.collab-sheet-editor__peer { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 50%; color: white; font-size: 11px; font-weight: 600; }
.collab-sheet-editor__loading { padding: 24px; }
.collab-sheet-editor__table-wrap { flex: 1; overflow: auto; padding: 16px; }
.collab-sheet-editor__grid { border-collapse: collapse; min-width: 100%; }
.collab-sheet-editor__colhead, .collab-sheet-editor__rowhead { background: var(--td-bg-color-secondarycontainer); padding: 6px 8px; font-weight: 500; min-width: 80px; border: 1px solid var(--td-component-stroke); }
.collab-sheet-editor__grid td { border: 1px solid var(--td-component-stroke); padding: 0; position: relative; }
.collab-sheet-editor__cell-lock {
  position: absolute;
  top: 1px;
  left: 2px;
  font-size: 10px;
  z-index: 4;
  pointer-events: none;
  opacity: 0.85;
}
.collab-sheet-editor__cell--locked { background: rgba(255, 200, 200, 0.3); cursor: not-allowed; }
.collab-sheet-editor__peer-label {
  position: absolute;
  top: -1px;
  right: -1px;
  padding: 1px 6px;
  font-size: 10px;
  color: #fff;
  border: 1px solid;
  border-radius: 6px 0 4px 0;
  pointer-events: none;
  z-index: 4;
  font-weight: 500;
  letter-spacing: 0.2px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.15);
  white-space: nowrap;
  max-width: 90px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.collab-sheet-editor__cell-input, .collab-sheet-editor__header-input { width: 100%; padding: 6px 8px; border: none; outline: none; background: transparent; }
.collab-sheet-editor__cell-input:focus, .collab-sheet-editor__header-input:focus { background: var(--td-brand-color-1); }

/* v0.7.28 — sheet tabs + formula bar */
.collab-sheet-editor__tabs {
  display: flex;
  gap: 2px;
  padding: 4px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  overflow-x: auto;
}
.collab-sheet-editor__tab {
  padding: 4px 12px;
  font-size: 12px;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 4px 4px 0 0;
  cursor: pointer;
  color: var(--td-text-color-secondary);
}
.collab-sheet-editor__tab.active {
  background: var(--td-bg-color-container);
  border-color: var(--td-component-stroke);
  border-bottom-color: var(--td-bg-color-container);
  color: var(--td-text-color-primary);
  font-weight: 600;
  margin-bottom: -1px;
}
.collab-sheet-editor__formula {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
}
.collab-sheet-editor__cellref {
  min-width: 56px;
  font-weight: 600;
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
.collab-sheet-editor__fx {
  font-style: italic;
  font-weight: 700;
  color: var(--td-brand-color-7);
}
.collab-sheet-editor__formula-input {
  flex: 1;
  padding: 4px 8px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 4px;
  font-family: 'JetBrains Mono', ui-monospace, monospace;
  font-size: 13px;
}
.collab-sheet-editor__formula-error {
  color: var(--td-error-color-7);
  font-size: 12px;
}
.collab-sheet-editor__formula-hint {
  color: var(--td-brand-color-7);
  font-size: 12px;
  font-family: 'JetBrains Mono', ui-monospace, monospace;
}
.collab-sheet-editor__cell--selected {
  outline: 2px solid var(--td-brand-color-7);
  outline-offset: -2px;
}
.collab-sheet-editor__feature { background: #f0f4f8; border: 1px solid #d0d7de; padding: 2px 8px; border-radius: 4px; cursor: pointer; }
.collab-sheet-editor__feature:hover { background: #e6edf3; }
.collab-sheet-editor__modal-bg { position: fixed; inset: 0; background: rgba(0,0,0,0.4); display: flex; align-items: center; justify-content: center; z-index: 1000; }
.collab-sheet-editor__modal { background: white; padding: 20px; border-radius: 8px; min-width: 360px; max-width: 90vw; box-shadow: 0 4px 20px rgba(0,0,0,0.15); }
.collab-sheet-editor__modal h3 { margin-top: 0; font-size: 16px; }
.collab-sheet-editor__modal label { display: block; margin: 8px 0; font-size: 14px; }
.collab-sheet-editor__modal input, .collab-sheet-editor__modal select { margin-left: 8px; padding: 2px 6px; border: 1px solid #d0d7de; border-radius: 4px; }
.collab-sheet-editor__modal-actions { display: flex; gap: 8px; margin-top: 16px; justify-content: flex-end; }
.collab-sheet-editor__modal-actions button { padding: 4px 12px; border: 1px solid #d0d7de; border-radius: 4px; background: #f6f8fa; cursor: pointer; }
.collab-sheet-editor__modal-actions button:hover { background: #eaeef2; }
.collab-sheet-editor__modal-actions button:last-child { background: #2da44e; color: white; border-color: #2c974b; }
.collab-sheet-editor__modal-actions button:last-child:hover { background: #2c974b; }

</style>

.collab-sheet-editor__names-list { list-style: none; padding: 0; margin: 0 0 12px; max-height: 180px; overflow-y: auto; }
.collab-sheet-editor__names-list li { display: flex; align-items: center; gap: 8px; padding: 4px 0; border-bottom: 1px solid var(--td-component-stroke); }
.collab-sheet-editor__names-name { font-weight: 600; color: var(--td-brand-color-7); min-width: 80px; }
.collab-sheet-editor__names-formula { flex: 1; color: var(--td-text-color-secondary); font-family: 'JetBrains Mono', ui-monospace, monospace; font-size: 12px; }
.collab-sheet-editor__names-del { background: transparent; border: 1px solid var(--td-component-stroke); color: var(--td-error-color); border-radius: 4px; cursor: pointer; padding: 2px 8px; font-size: 12px; }
.collab-sheet-editor__names-empty { color: var(--td-text-color-secondary); margin: 0 0 12px; font-size: 13px; }
.collab-sheet-editor__names-add { display: flex; flex-direction: column; gap: 8px; padding-top: 8px; border-top: 1px solid var(--td-component-stroke); }
.collab-sheet-editor__names-add label { display: flex; align-items: center; gap: 6px; font-size: 12px; }
.collab-sheet-editor__cell-input.is-formula { color: var(--td-brand-color-7); font-family: 'JetBrains Mono', ui-monospace, monospace; }
.collab-sheet-editor__cell-input.is-percent { text-align: right; }
