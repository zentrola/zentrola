// 触发浏览器保存 Blob：锚点需挂到文档中才能在所有浏览器触发下载，
// 对象 URL 延迟释放，避免下载尚未开始就被撤销。
export function saveBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = name
  anchor.style.display = 'none'
  document.body.append(anchor)
  anchor.click()
  anchor.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
