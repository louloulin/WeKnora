/**
 * slideGroupSync — v0.7.108 跨客户端 group 投影
 *
 * 把 Yjs 端 shape.groupId 字段变化投影到 engine 端 slide.elements。
 * Yjs 端 groupId 是 `g_<rand>_<ts>`（Yjs 维度的 group id）；engine 端
 * groupElements() 返回的是它内部的 element id（`e_<guid8>`）。
 *
 * 调用方（CollabSlideKonvaEditor 的 syncFromY）每次 Yjs 状态变更都把当前
 * slide 的 shapes 喂进来；本模块比对 lastGroups 与 currentGroups：
 *   - 新出现 gid → 调 groupElements(对应 sourceIds) 并把返回的 engineGid 写入引擎映射
 *   - 消失 gid → 调 ungroupElement(对应 engineGid) 并清掉映射
 *
 * 本地 groupSelected / ungroupSelected 也调一次 engine，本模块通过
 * markLocalGrouped / markLocalUngrouped 把 gid 登记到 lastGroups，让紧随的
 * syncFromY 看到"无 diff"避免重复触发。
 */
import {
  groupElements,
  ungroupElement,
} from '../engines/pptx-engine/index'
import type { OpenedPptx } from '../engines/pptx-engine/types'
import type { PptxShape } from './pptxShapeAdapter'

/** Track which Yjs groupIds have been projected per slide. */
export type ProjectionState = Map<number, Set<string>>

/** Map Yjs groupId -> engine group element id (so we can ungroup later). */
export type EngineGroupMap = Record<string, string>

export interface ProjectOptions {
  /** Yjs-driven shapes on a single slide. */
  shapes: PptxShape[]
  slideIdx: number
  opened: OpenedPptx
  state: ProjectionState
  engineMap: EngineGroupMap
}

/** Returns the diff that was applied (mostly for tests / debug). */
export interface ProjectDiff {
  grouped: string[]
  ungrouped: string[]
}

export function projectGroupsToEngine(opts: ProjectOptions): ProjectDiff {
  const { shapes, slideIdx, opened, state, engineMap } = opts
  const currentGids = new Set<string>()
  const gidToSourceIds = new Map<string, string[]>()
  for (const sh of shapes) {
    if (!sh.groupId) continue
    currentGids.add(sh.groupId)
    let arr = gidToSourceIds.get(sh.groupId)
    if (!arr) { arr = []; gidToSourceIds.set(sh.groupId, arr) }
    arr.push(sh.id)
  }
  const lastGids = state.get(slideIdx) ?? new Set<string>()
  const diff: ProjectDiff = { grouped: [], ungrouped: [] }
  for (const gid of currentGids) {
    if (lastGids.has(gid)) continue
    const sourceIds = gidToSourceIds.get(gid) ?? []
    if (sourceIds.length < 2) continue
    try {
      const result = groupElements(opened, slideIdx, sourceIds)
      if (result) {
        engineMap[gid] = result.groupId
        diff.grouped.push(gid)
      }
    } catch (e) {
      console.warn('[slideGroupSync] groupElements failed', e)
    }
  }
  for (const gid of lastGids) {
    if (currentGids.has(gid)) continue
    const engineGid = engineMap[gid]
    if (!engineGid) continue
    try {
      ungroupElement(opened, slideIdx, engineGid)
      diff.ungrouped.push(gid)
    } catch (e) {
      console.warn('[slideGroupSync] ungroupElement failed', e)
    }
    delete engineMap[gid]
  }
  state.set(slideIdx, currentGids)
  return diff
}

/** Record a local groupSelected() so the next syncFromY sees no diff. */
export function markLocalGrouped(state: ProjectionState, slideIdx: number, gid: string) {
  const set = state.get(slideIdx) ?? new Set<string>()
  set.add(gid)
  state.set(slideIdx, set)
}

/** Record a local ungroupSelected() so the next syncFromY sees no diff. */
export function markLocalUngrouped(state: ProjectionState, slideIdx: number) {
  state.delete(slideIdx)
}
