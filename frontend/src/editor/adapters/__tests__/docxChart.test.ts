/**
 * v0.7.32 — DOC embedded chart part smoke test.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  buildDocChartPart,
  patchDocChartPart,
  parseDocChartPart,
} from '../docxAdapter'

test('buildDocChartPart produces a chartN.xml part for a bar chart', () => {
  const xml = buildDocChartPart({
    kind: 'bar',
    title: '示例柱状图',
    categories: ['Jan', 'Feb', 'Mar'],
    series: [
      { name: '本部门', values: [12, 18, 9] },
      { name: '行业均值', values: [10, 14, 11] },
    ],
  })
  assert.ok(xml.length > 100, 'chart XML should be non-trivial')
  assert.match(xml, /^<\?xml/, 'should start with XML decl')
  assert.match(xml, /c:chartSpace/, 'should contain c:chartSpace root')
  assert.match(xml, /c:barChart/, 'should contain c:barChart for kind=bar')
})

test('buildDocChartPart produces a c:lineChart for kind=line', () => {
  const xml = buildDocChartPart({
    kind: 'line',
    title: '示例折线图',
    categories: ['Jan', 'Feb', 'Mar'],
    series: [{ name: '趋势', values: [5, 7, 6] }],
  })
  assert.match(xml, /c:lineChart/, 'should contain c:lineChart for kind=line')
})

test('buildDocChartPart produces a c:pieChart for kind=pie', () => {
  const xml = buildDocChartPart({
    kind: 'pie',
    title: '份额',
    categories: ['A', 'B', 'C'],
    series: [{ name: '份额', values: [40, 35, 25] }],
  })
  assert.match(xml, /c:pieChart/, 'should contain c:pieChart for kind=pie')
})

test('patchDocChartPart updates the chart title in-place', () => {
  const xml = buildDocChartPart({
    kind: 'bar',
    title: '原标题',
    categories: ['Jan'],
    series: [{ name: 'series', values: [10] }],
  })
  const patched = patchDocChartPart(xml, { title: '新标题' })
  assert.match(patched, /新标题/)
})

test('parseDocChartPart round-trips the build → parse', () => {
  const xml = buildDocChartPart({
    kind: 'bar',
    title: '回环测试',
    categories: ['Jan', 'Feb'],
    series: [{ name: 's', values: [1, 2] }],
  })
  const parsed = parseDocChartPart(xml)
  assert.ok(parsed, 'parser should return a model')
  assert.equal(parsed!.kind, 'bar')
  assert.equal(parsed!.title, '回环测试')
  assert.deepEqual(parsed!.categories, ['Jan', 'Feb'])
  assert.equal(parsed!.series.length, 1)
})
