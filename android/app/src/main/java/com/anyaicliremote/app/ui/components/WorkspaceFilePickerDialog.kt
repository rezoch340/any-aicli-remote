package com.anyaicliremote.app.ui.components

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.anyaicliremote.app.model.WorkspaceFile
import com.anyaicliremote.app.ui.ChatUiState

@Composable
internal fun WorkspaceFilePickerDialog(
    state: ChatUiState,
    onDismiss: () -> Unit,
    onBrowseWorkspace: (String) -> Unit,
    onToggleFile: (WorkspaceFile) -> Unit,
) {
    val selectedPaths = state.selectedFiles.map { it.path }.toSet()
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("选择工作区文件") },
        text = {
            Column {
                Text(
                    state.filePickerPath,
                    modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                    maxLines = 1,
                    softWrap = false,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (state.filePickerLoading) {
                    CircularProgressIndicator(Modifier.padding(24.dp).align(Alignment.CenterHorizontally))
                } else if (state.filePickerError != null) {
                    Text(state.filePickerError, modifier = Modifier.padding(vertical = 16.dp))
                } else {
                    LazyColumn(Modifier.heightIn(max = 420.dp), verticalArrangement = Arrangement.spacedBy(5.dp)) {
                        state.filePickerParent?.let { parent ->
                            item("parent") {
                                TextButton(onClick = { onBrowseWorkspace(parent) }) { Text("返回上级") }
                            }
                        }
                        items(state.filePickerDirectories, key = { "directory:${it.path}" }) { file ->
                            FileCard(
                                file = file,
                                modifier = Modifier.fillMaxWidth(),
                                onClick = { onBrowseWorkspace(file.path) },
                            )
                        }
                        items(state.filePickerFiles, key = { "file:${it.path}" }) { file ->
                            FileCard(
                                file = file,
                                modifier = Modifier.fillMaxWidth(),
                                selected = file.path in selectedPaths,
                                onClick = { onToggleFile(file) },
                            )
                        }
                        if (state.filePickerDirectories.isEmpty() && state.filePickerFiles.isEmpty()) {
                            item("empty") { Text("此目录没有可选文件", modifier = Modifier.padding(vertical = 20.dp)) }
                        }
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("完成") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}
