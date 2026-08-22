package com.grokremote.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

private data class CompactTableCell(val text: String, val code: Boolean = false)
private data class CompactTableData(
    val header: List<CompactTableCell>?,
    val rows: List<List<CompactTableCell>>,
    val columnCount: Int,
)

@Composable
internal fun CompactMarkdownTable(markdown: String) {
    val table = remember(markdown) { parseCompactTable(markdown) }
    if (table.columnCount == 0) return
    val compactFirstColumn = table.columnCount == 2 &&
        (listOfNotNull(table.header) + table.rows).all { row -> row.firstOrNull()?.text.orEmpty().length <= 4 }
    val shape = RoundedCornerShape(12.dp)

    if (table.columnCount > 2) {
        CompactRecordTable(table, shape)
        return
    }

    SelectionContainer {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(vertical = 5.dp)
                .border(1.dp, MaterialTheme.colorScheme.outline.copy(alpha = 0.7f), shape)
                .clip(shape)
                .background(Color(0xFF121317)),
        ) {
            table.header?.let { header ->
                CompactTableRow(
                    cells = header,
                    columnCount = table.columnCount,
                    header = true,
                    compactFirstColumn = compactFirstColumn,
                )
                if (table.rows.isNotEmpty()) {
                    HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.7f))
                }
            }
            table.rows.forEachIndexed { index, row ->
                CompactTableRow(
                    cells = row,
                    columnCount = table.columnCount,
                    header = false,
                    compactFirstColumn = compactFirstColumn,
                )
                if (index != table.rows.lastIndex) {
                    HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.4f))
                }
            }
        }
    }
}

@Composable
private fun CompactRecordTable(table: CompactTableData, shape: RoundedCornerShape) {
    SelectionContainer {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(vertical = 5.dp)
                .border(1.dp, MaterialTheme.colorScheme.outline.copy(alpha = 0.7f), shape)
                .clip(shape)
                .background(Color(0xFF121317)),
        ) {
            table.rows.forEachIndexed { rowIndex, row ->
                Column(Modifier.fillMaxWidth().padding(horizontal = 13.dp, vertical = 12.dp)) {
                    Surface(
                        color = Color(0xFF24252A),
                        shape = RoundedCornerShape(7.dp),
                    ) {
                        Text(
                            row.firstOrNull()?.text?.ifBlank { "#${rowIndex + 1}" } ?: "#${rowIndex + 1}",
                            modifier = Modifier.padding(horizontal = 9.dp, vertical = 4.dp),
                            style = MaterialTheme.typography.labelSmall,
                            fontWeight = FontWeight.Bold,
                            color = MaterialTheme.colorScheme.onSurface,
                        )
                    }
                    Spacer(Modifier.height(11.dp))

                    for (column in 1 until table.columnCount) {
                        val cell = row.getOrNull(column) ?: continue
                        if (cell.text.isBlank()) continue
                        val label = table.header?.getOrNull(column)?.text.orEmpty()
                        if (label.isNotBlank()) {
                            Text(
                                label,
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            Spacer(Modifier.height(3.dp))
                        }
                        Text(
                            cell.text,
                            style = MaterialTheme.typography.bodySmall.copy(
                                fontFamily = if (cell.code || label.contains("id", ignoreCase = true) || label.contains("种子")) {
                                    FontFamily.Monospace
                                } else {
                                    FontFamily.Default
                                },
                                lineHeight = 18.sp,
                            ),
                            color = MaterialTheme.colorScheme.onSurface,
                            softWrap = true,
                        )
                        if (column != table.columnCount - 1) Spacer(Modifier.height(10.dp))
                    }
                }
                if (rowIndex != table.rows.lastIndex) {
                    HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.45f))
                }
            }
        }
    }
}

@Composable
private fun CompactTableRow(
    cells: List<CompactTableCell>,
    columnCount: Int,
    header: Boolean,
    compactFirstColumn: Boolean,
) {
    val normalized = List(columnCount) { cells.getOrNull(it) ?: CompactTableCell("") }
    if (header && compactFirstColumn && normalized.first().text.isBlank()) {
        Box(
            Modifier.fillMaxWidth().background(Color(0xFF17181C)).padding(horizontal = 12.dp, vertical = 11.dp),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                normalized[1].text,
                style = MaterialTheme.typography.bodySmall,
                fontWeight = FontWeight.SemiBold,
                color = MaterialTheme.colorScheme.onSurface,
            )
        }
        return
    }

    Row(
        Modifier
            .fillMaxWidth()
            .background(if (header) Color(0xFF17181C) else Color.Transparent),
        verticalAlignment = Alignment.Top,
    ) {
        normalized.forEachIndexed { index, cell ->
            val cellModifier = if (compactFirstColumn && index == 0) {
                Modifier.width(48.dp)
            } else {
                Modifier.weight(if (compactFirstColumn) 1f else compactTableColumnWeight(normalized, index))
            }
            Text(
                text = cell.text,
                modifier = cellModifier.padding(horizontal = 12.dp, vertical = 10.dp),
                style = MaterialTheme.typography.bodySmall.copy(
                    fontFamily = if (cell.code) FontFamily.Monospace else FontFamily.Default,
                    lineHeight = 18.sp,
                ),
                fontWeight = if (header) FontWeight.SemiBold else FontWeight.Normal,
                color = if (cell.code) Color(0xFFD6D7DA) else MaterialTheme.colorScheme.onSurface,
                softWrap = true,
            )
        }
    }
}

private fun compactTableColumnWeight(row: List<CompactTableCell>, index: Int): Float {
    val length = row.getOrNull(index)?.text.orEmpty().length.coerceIn(4, 24)
    return length.toFloat()
}

private fun parseCompactTable(markdown: String): CompactTableData {
    val parsedRows = markdown.lineSequence()
        .map(String::trim)
        .filter(String::isNotEmpty)
        .map(::splitMarkdownTableRow)
        .filter { it.isNotEmpty() }
        .toList()
    if (parsedRows.isEmpty()) return CompactTableData(null, emptyList(), 0)

    val separator = parsedRows.indexOfFirst { row ->
        row.isNotEmpty() && row.all { it.text.matches(Regex(":?-{3,}:?")) }
    }
    val header = if (separator > 0) parsedRows[separator - 1] else null
    val rows = if (separator >= 0) parsedRows.drop(separator + 1) else parsedRows
    val columnCount = (listOfNotNull(header) + rows).maxOfOrNull(List<CompactTableCell>::size) ?: 0
    return CompactTableData(header, rows, columnCount)
}

private fun splitMarkdownTableRow(line: String): List<CompactTableCell> {
    var source = line.trim()
    if (source.startsWith('|')) source = source.drop(1)
    if (source.endsWith('|') && !source.endsWith("\\|")) source = source.dropLast(1)

    val rawCells = mutableListOf<String>()
    val cell = StringBuilder()
    var escaped = false
    var inCode = false
    source.forEach { char ->
        when {
            escaped -> {
                cell.append(char)
                escaped = false
            }
            char == '\\' -> escaped = true
            char == '`' -> {
                inCode = !inCode
                cell.append(char)
            }
            char == '|' && !inCode -> {
                rawCells += cell.toString()
                cell.clear()
            }
            else -> cell.append(char)
        }
    }
    if (escaped) cell.append('\\')
    rawCells += cell.toString()

    return rawCells.map { raw ->
        val trimmed = raw.trim()
        val code = trimmed.length >= 2 && trimmed.startsWith('`') && trimmed.endsWith('`')
        CompactTableCell(
            text = if (code) trimmed.drop(1).dropLast(1).trim() else trimmed,
            code = code,
        )
    }
}
