import { useEffect } from 'react'
import { X } from 'lucide-react'

interface ConfirmDialogProps {
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  onConfirm: () => void
  onCancel: () => void
  danger?: boolean
}

export function ConfirmDialog({
  title,
  message,
  confirmLabel = 'Confirmar',
  cancelLabel = 'Cancelar',
  onConfirm,
  onCancel,
  danger = false,
}: ConfirmDialogProps) {
  useEffect(() => {
    function handleKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onCancel()
      if (e.key === 'Enter') onConfirm()
    }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [onConfirm, onCancel])

  return (
    <div className="confirm-backdrop" onClick={(e) => { if (e.target === e.currentTarget) onCancel() }}>
      <div className="confirm-dialog" role="alertdialog">
        <div className="confirm-header">
          <span className="confirm-title">{title}</span>
          <button className="confirm-close" onClick={onCancel}><X size={12} /></button>
        </div>
        <div className="confirm-body">
          <p className="confirm-message">{message}</p>
        </div>
        <div className="confirm-footer">
          <button className="confirm-btn confirm-btn--cancel" onClick={onCancel}>{cancelLabel}</button>
          <button
            className={`confirm-btn confirm-btn--confirm${danger ? ' confirm-btn--danger' : ''}`}
            onClick={onConfirm}
            autoFocus
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
