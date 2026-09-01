/**
 * docProtection — DOC document protection UI helper (v0.7.67).
 *
 * Vendored from genoffice apps/docs/src/renderer/components/ProtectDialog.tsx
 * (Word for Mac "Protect Document" sheet). The Vue 3 surface stays minimal:
 *
 *   - protection options live in component ref state (DocProtectionKind, password,
 *     hash, salt, spinCount, algorithmSid)
 *   - `applyDocProtection(currentProtection, newState)` returns the resulting
 *     DocProtection to pass to saveDocx's options.protection, OR null to remove
 *     the restriction.
 *   - `needsUnlock(currentProtection, newState)` decides whether the user must
 *     re-enter the existing password before changing a locked restriction.
 *
 * Adapted:
 *   - React useState → plain JSON patch object (caller owns the reactive state).
 *   - hashProtectionPassword / verifyProtectionPassword come from docx-engine.
 */
import {
  hashProtectionPassword,
  verifyProtectionPassword,
  type DocProtection,
  type WriteProtection,
} from '../engines/docx-engine/index'

export const PROTECTION_MODES = [
  'trackedChanges',
  'comments',
  'readOnly',
  'forms',
] as const
export type ProtectionMode = (typeof PROTECTION_MODES)[number]

/** KEEP sentinel — pre-filled into password fields that already hold a hash so the user can choose to keep the existing password. */
export const PROTECTION_KEEP = '\u0001'.repeat(8)

export interface DocProtectionPatch {
  /** editing mode (Word's w:edit values) */
  mode: ProtectionMode
  /** true = w:enforcement="1" (the restriction is active) */
  enabled: boolean
  /** new password to set; '' = no password; PROTECTION_KEEP = keep existing */
  password: string
  /** confirm password */
  passwordConfirm: string
  /** when changing a locked restriction: the user must type the existing password */
  unlockPassword: string
  /** error message key ('' = none); translated by the caller */
  error: string
  /** async submit in flight; caller disables the submit button */
  busy: boolean
}

export function makeProtectionPatch(current: DocProtection | null): DocProtectionPatch {
  const locked = !!current?.enforced && !!current?.hash
  return {
    mode: (PROTECTION_MODES as readonly string[]).includes(current?.edit ?? '')
      ? (current?.edit as ProtectionMode)
      : 'trackedChanges',
    enabled: !!current?.enforced,
    password: locked ? PROTECTION_KEEP : '',
    passwordConfirm: locked ? PROTECTION_KEEP : '',
    unlockPassword: '',
    error: '',
    busy: false,
  }
}

/** Apply the patch to a current DocProtection (if any). Returns the new value, or null to remove. */
export async function applyDocProtection(
  current: DocProtection | null,
  patch: DocProtectionPatch,
): Promise<{ protection: DocProtection | null; error: string }> {
  if (patch.password !== patch.passwordConfirm) {
    return { protection: current, error: 'pwdMismatch' }
  }
  const wasLocked = !!current?.enforced && !!current?.hash
  if (wasLocked && !patch.enabled) {
    // turning off an enforced restriction: need the existing password
    if (!(await verifyProtectionPassword(patch.unlockPassword, current!))) {
      return { protection: current, error: 'wrongUnlock' }
    }
  }
  if (!patch.enabled) {
    return { protection: null, error: '' }
  }
  if (wasLocked && patch.password === PROTECTION_KEEP) {
    // unchanged password, keep the existing credentials
    return { protection: { ...current!, edit: patch.mode, enforced: true }, error: '' }
  }
  const creds =
    patch.password && patch.password !== PROTECTION_KEEP
      ? await hashProtectionPassword(patch.password)
      : {}
  return {
    protection: { edit: patch.mode, enforced: true, ...creds },
    error: '',
  }
}

/** A password-to-modify restriction (settings.xml w:writeProtection) — honor system, no encryption. */
export interface WriteProtectionPatch {
  /** true = w:recommended="1" (Word suggests opening read-only) */
  recommended: boolean
  /** new password; '' = remove; PROTECTION_KEEP = keep existing */
  password: string
  passwordConfirm: string
  error: string
}

export function makeWriteProtectionPatch(current: WriteProtection | null): WriteProtectionPatch {
  const had = !!current?.hash
  return {
    recommended: current?.recommended ?? false,
    password: had ? PROTECTION_KEEP : '',
    passwordConfirm: had ? PROTECTION_KEEP : '',
    error: '',
  }
}

export async function applyWriteProtection(
  current: WriteProtection | null,
  patch: WriteProtectionPatch,
): Promise<{ writeProtection: WriteProtection | null; error: string }> {
  if (patch.password !== patch.passwordConfirm) {
    return { writeProtection: current, error: 'pwdMismatch' }
  }
  const had = !!current?.hash
  if (patch.password === '') {
    return {
      writeProtection: patch.recommended ? { recommended: true } : null,
      error: '',
    }
  }
  if (patch.password === PROTECTION_KEEP && had) {
    return {
      writeProtection: { ...current!, recommended: patch.recommended },
      error: '',
    }
  }
  const creds = await hashProtectionPassword(patch.password)
  return {
    writeProtection: { recommended: patch.recommended, ...creds },
    error: '',
  }
}

/** Chinese translations for the protection dialog (minimal — the UI is fully Chinese otherwise). */
export const PROTECTION_I18N: Record<string, string> = {
  pwdMismatch: '两次输入的密码不一致',
  wrongUnlock: '现有密码不正确',
  pwdTooShort: '密码至少需要 6 位',
}

export function validateProtectionPatch(patch: DocProtectionPatch): string {
  if (patch.enabled && patch.password && patch.password !== PROTECTION_KEEP) {
    if (patch.password.length < 6) return 'pwdTooShort'
  }
  return ''
}
