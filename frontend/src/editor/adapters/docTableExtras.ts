/**
 * v0.7.54 — TipTap table node extensions carrying the Word table-property
 * attrs that genoffice's table-properties.ts commands write (copy of the
 * attr surface from genoffice extensions.ts docTable/docTableCell/docTableRow).
 *
 * The base @tiptap/extension-table nodes only model colspan/rowspan/colwidth;
 * these extends add the OOXML-facing attrs so the copied commands and the
 * pmTableToTableXml save path can round-trip borders/fills/autofit/headers.
 */
import Table from '@tiptap/extension-table'
import TableRow from '@tiptap/extension-table-row'
import TableCell from '@tiptap/extension-table-cell'
import TableHeader from '@tiptap/extension-table-header'

export interface TableBorderSpec {
  style: string
  szEighths: number
  color: string
}

export const DocTable = Table.extend({
  addAttributes() {
    return {
      tblAutoFit: { default: null as string | null },
      tblAutoFitEdited: { default: false },
      widthPx: { default: null as number | null },
      widthPct: { default: null as number | null },
      colWidthsPct: { default: null as number[] | null },
      tblLook: { default: null as Record<string, boolean> | null },
      tblLookEdited: { default: false },
      tblStyleId: { default: null as string | null },
    }
  },
})

export const DocTableRow = TableRow.extend({
  addAttributes() {
    return {
      repeatHeader: { default: false },
      repeatHeaderEdited: { default: false },
    }
  },
})

export const DocTableCell = TableCell.extend({
  addAttributes() {
    return {
      fill: { default: null as string | null },
      borders: { default: null as Record<string, TableBorderSpec> | null },
    }
  },
})

export const DocTableHeader = TableHeader.extend({
  addAttributes() {
    return {
      fill: { default: null as string | null },
      borders: { default: null as Record<string, TableBorderSpec> | null },
    }
  },
})
