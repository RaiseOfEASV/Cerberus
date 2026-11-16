import { describe, expect, it } from 'vitest'
import { can, canOnFinding, nextStatuses, type Action, type Role } from './permissions'

const roles: Role[] = ['analyst', 'developer', 'pm', 'auditor']

describe('permission matrix mirrors the server (domain.go)', () => {
  const want: Record<Action, [boolean, boolean, boolean, boolean]> = {
    view: [true, true, true, true],
    create: [true, false, true, false],
    import: [true, false, true, false],
    edit: [true, true, true, false],
    change_status: [true, true, true, false],
    assign: [true, false, true, false],
    link: [true, true, true, false],
    attach: [true, true, true, false],
    export: [true, false, true, true],
    view_audit: [false, false, true, true],
    set_sla: [false, false, true, false],
    manage_roles: [false, false, true, false],
  }
  for (const [action, row] of Object.entries(want) as [Action, boolean[]][]) {
    it(action, () => {
      roles.forEach((r, i) => expect(can(r, action), `${r} ${action}`).toBe(row[i]))
    })
  }
})

describe('ownership and transitions', () => {
  it('developers only act on their own findings', () => {
    expect(canOnFinding('developer', 'edit', 'u1', 'u2')).toBe(false)
    expect(canOnFinding('developer', 'edit', 'u1', 'u1')).toBe(true)
    expect(canOnFinding('developer', 'edit', 'u1', '')).toBe(false)
    expect(canOnFinding('analyst', 'edit', 'u1', 'u2')).toBe(true)
  })

  it('offers only legal next states per role', () => {
    expect(nextStatuses('analyst', 'New')).toEqual(['Triaged', 'Accepted Risk'])
    expect(nextStatuses('developer', 'Triaged')).toEqual(['In Progress'])
    expect(nextStatuses('developer', 'In Progress')).toEqual(['Remediated'])
    expect(nextStatuses('pm', 'Remediated')).toEqual(['In Progress'])
    expect(nextStatuses('pm', 'Accepted Risk')).toEqual(['Triaged'])
    expect(nextStatuses('auditor', 'New')).toEqual([])
  })
})
