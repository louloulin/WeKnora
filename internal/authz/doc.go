// Package authz is the relationship-based authorisation layer for WeKnora.
//
// It unifies the previously scattered permission checks (RBAC role matrix,
// KB ownership, Wiki page ACL, Agent Share, KB Share, API-key capabilities)
// behind a single Check(user, object, relation) API. The model is modelled
// after OpenFGA's Object#relation@user tuples, but the storage is local to
// the process so the hot path stays sub-millisecond.
//
// Why this exists:
//   - The route layer currently uses one RBAC guard per route plus
//     per-handler ownership checks; the same user can pass a guard and
//     then get a 403 from the service layer because the guard and the
//     service consult different stores.
//   - Wiki ACL has its own cache + decision vocabulary (allow /
//     deny_allow_list / deny_private) that does not compose with the
//     KB Share API; admins cannot audit a single deny across both.
//   - KB.Owner is a single hard-coded column, but Agent Share already
//     supports per-relation grants; the inconsistency blocks any
//     future "Org Manager" or "Workspace Owner" relation from landing.
//
// What ships in this package today:
//   - Object / User / Relation / Decision / CheckRequest value types.
//   - A Checker interface with Check / CheckBulk / Invalidate.
//   - A composite Checker that fans out to per-object-type adapters
//     (KB, wiki page, agent, tenant role) and merges decisions.
//   - A reason-code vocabulary that is stable across upgrades and is
//     safe to surface in audit logs and admin dashboards.
//
// What ships in a follow-up:
//   - Persistence-backed tuple store (today the adapters read live
//     from the existing services, which is enough for the hot path;
//     a snapshot/replay layer is needed only once we want historical
//     "who could see X on date Y" queries).
//   - Cross-instance consistency via the existing event bus.
//   - Admin UI for visualising and editing the relation graph.
package authz
