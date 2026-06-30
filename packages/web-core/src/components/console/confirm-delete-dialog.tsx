"use client";

import { Trash2Icon } from "lucide-react";
import { useState } from "react";
import { Button } from "@noxaaa/prism-oss-web-core/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@noxaaa/prism-oss-web-core/ui/dialog";
import { useI18n } from "@noxaaa/prism-oss-web-core/console/i18n";

export function ConfirmDeleteDialog({
  label,
  open,
  onConfirm,
  onOpenChange,
}: {
  label: string;
  open: boolean;
  onConfirm: () => Promise<void>;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  async function confirm() {
    setBusy(true);
    try {
      await onConfirm();
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("common.delete")}</DialogTitle>
          <DialogDescription>{label ? t("common.deleteQuestion", { name: label }) : t("common.deleteThisQuestion")}</DialogDescription>
        </DialogHeader>
        <DialogFooter showCloseButton>
          <Button disabled={busy} onClick={() => void confirm()} type="button" variant="destructive">
            <Trash2Icon data-icon="inline-start" />
            {t("common.delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
