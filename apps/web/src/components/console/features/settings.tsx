"use client";

import {
  ActivityIcon,
  CopyIcon,
  DownloadIcon,
  FileJsonIcon,
  MoreHorizontalIcon,
  NetworkIcon,
  PlusIcon,
  RadarIcon,
  RefreshCwIcon,
  RouteIcon,
  ServerIcon,
  ShieldIcon,
  TargetIcon,
  UploadIcon,
  UsersIcon,
} from "lucide-react";
import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldSet, FieldLegend } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { bytes, controlDelete, controlGet, controlPatch, controlPost, optionLabel, shortDate } from "@/components/console/control-api";
import { localizeControlError, useI18n } from "@/components/console/i18n";
import { hasPermission } from "@/components/console/permissions";
import { ResourceMultiSelect, ResourceSelect } from "@/components/console/resource-select";
import { useConsoleSession } from "@/components/console/shell";
import {
  ControlledTextField,
  DataState,
  EnumSelect,
  PageStack,
  ResourceTable,
  StatusBadge,
  SummaryCard,
  SummaryGrid,
  TextAreaField,
  TextField,
  copyText,
  duration,
  ensureFirstValue,
  groupName,
  monitorGroupName,
  percent,
  useControlList,
} from "@/components/console/shared";
import { cn } from "@/lib/utils";
import type {
  AgentMetrics,
  Member,
  Monitor,
  MonitorGroup,
  NodeGroup,
  NodeResource,
  RegistrationToken,
  ResourceOption,
  Role,
  Rule,
  RuleExportPayload,
  RuleImportResult,
  RuleTraffic,
  Target,
  TargetGroup,
} from "@/components/console/types";

export function SettingsPage() {
  const { t } = useI18n();
  const { session, refresh } = useConsoleSession();
  const canUpdateOrganization = hasPermission(session, "organization.update");
  const [editOpen, setEditOpen] = useState(false);

  async function handleUpdated() {
    await refresh();
    setEditOpen(false);
  }

  return (
    <PageStack>
      <OrganizationEditDrawer
        onOpenChange={setEditOpen}
        onUpdated={handleUpdated}
        open={canUpdateOrganization && editOpen}
        organization={session?.organization}
      />
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.organization")}</CardTitle>
          <CardDescription>{session?.organization?.id}</CardDescription>
          {canUpdateOrganization ? (
            <CardAction>
              <Button onClick={() => setEditOpen(true)} type="button">
                <ShieldIcon data-icon="inline-start" />
                {t("settings.editOrganization")}
              </Button>
            </CardAction>
          ) : null}
        </CardHeader>
        <CardContent>
          <Table>
            <TableBody>
              <TableRow>
                <TableCell>{t("field.name")}</TableCell>
                <TableCell>{session?.organization?.name}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>{t("settings.slug")}</TableCell>
                <TableCell>{session?.organization?.slug}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </PageStack>
  );
}

function OrganizationEditDrawer({
  onOpenChange,
  onUpdated,
  open,
  organization,
}: {
  onOpenChange: (open: boolean) => void;
  onUpdated: () => Promise<void>;
  open: boolean;
  organization?: { id: string; name: string; slug: string };
}) {
  const { t } = useI18n();
  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("settings.editOrganization")}</SheetTitle>
          <SheetDescription>{t("settings.editOrganizationDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          <OrganizationEditForm onUpdated={onUpdated} organization={organization} />
        </div>
      </SheetContent>
    </Sheet>
  );
}

function OrganizationEditForm({ onUpdated, organization }: { onUpdated: () => Promise<void>; organization?: { id: string; name: string; slug: string } }) {
  const { locale, t } = useI18n();
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    try {
      await controlPatch("/api/control/organizations/current", {
        name: form.get("name"),
        slug: form.get("slug"),
      });
      toast.success(t("settings.organizationUpdated"));
      await onUpdated();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={submit}>
      <FieldGroup>
        <TextField defaultValue={organization?.name} label={t("field.name")} name="name" placeholder={t("settings.organizationNamePlaceholder")} />
        <TextField defaultValue={organization?.slug} label={t("settings.slug")} name="slug" placeholder="acme-network" />
      </FieldGroup>
      <Button type="submit"><ShieldIcon data-icon="inline-start" />{t("common.save")}</Button>
    </form>
  );
}
