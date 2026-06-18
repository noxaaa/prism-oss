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

export function MonitorsPage() {
  const { locale, t } = useI18n();
  const { session } = useConsoleSession();
  const canManage = hasPermission(session, "monitors.manage");
  const groups = useControlList<MonitorGroup>("/api/control/monitor-groups");
  const monitors = useControlList<Monitor>("/api/control/monitors");
  const [token, setToken] = useState<RegistrationToken | null>(null);
  const [monitorGroupCreateOpen, setMonitorGroupCreateOpen] = useState(false);
  const [monitorCreateOpen, setMonitorCreateOpen] = useState(false);
  const groupOptions = groups.data.map((group) => ({ value: group.id, label: group.name }));

  async function refreshAll() {
    await Promise.all([groups.refresh(), monitors.refresh()]);
  }

  async function createToken(monitorID: string) {
    try {
      const result = await controlPost<RegistrationToken>(`/api/control/monitors/${monitorID}/registration-token`, { ttl_hours: 24 });
      setToken(result);
      toast.success(t("monitors.registrationTokenCreated"));
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <PageStack>
      {canManage ? (
        <>
          <MonitorGroupCreateDrawer onCreated={refreshAll} onOpenChange={setMonitorGroupCreateOpen} open={monitorGroupCreateOpen} />
          <MonitorCreateDrawer groupOptions={groupOptions} onCreated={refreshAll} onOpenChange={setMonitorCreateOpen} open={monitorCreateOpen} />
          <div className="flex flex-wrap justify-end gap-2">
            <Button onClick={() => setMonitorGroupCreateOpen(true)} type="button" variant="outline">
              <PlusIcon data-icon="inline-start" />
              {t("monitors.createGroup")}
            </Button>
            <Button onClick={() => setMonitorCreateOpen(true)} type="button">
              <PlusIcon data-icon="inline-start" />
              {t("monitors.createMonitor")}
            </Button>
          </div>
        </>
      ) : null}

      {token?.install_command ? (
        <Alert>
          <FileJsonIcon />
          <AlertTitle>{t("monitors.installCommand")}</AlertTitle>
          <AlertDescription>
            <code className="block overflow-auto rounded-lg bg-muted p-3 text-xs">{token.install_command}</code>
          </AlertDescription>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>{t("monitors.monitors")}</CardTitle>
          <CardAction>
            <Button onClick={refreshAll} size="sm" type="button" variant="outline"><RefreshCwIcon data-icon="inline-start" />{t("common.refresh")}</Button>
          </CardAction>
        </CardHeader>
        <CardContent>
          <DataState loading={monitors.loading} error={monitors.error}>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("field.name")}</TableHead>
                  <TableHead>{t("overview.status")}</TableHead>
                  <TableHead>{t("monitors.groups")}</TableHead>
                  <TableHead>{t("monitors.config")}</TableHead>
                  <TableHead>{t("overview.lastSeen")}</TableHead>
                  {canManage ? <TableHead>{t("common.actions")}</TableHead> : null}
                </TableRow>
              </TableHeader>
              <TableBody>
                {monitors.data.map((monitor) => (
                  <TableRow key={monitor.id}>
                    <TableCell>{monitor.name}</TableCell>
                    <TableCell><StatusBadge value={monitor.status} /></TableCell>
                    <TableCell>{monitor.group_ids.map((id) => monitorGroupName(groups.data, id)).join(", ")}</TableCell>
                    <TableCell>{monitor.applied_config_version}/{monitor.desired_config_version}</TableCell>
                    <TableCell>{shortDate(monitor.last_seen_at, locale)}</TableCell>
                    {canManage ? <TableCell><Button onClick={() => createToken(monitor.id)} size="sm" type="button" variant="outline">{t("monitors.token")}</Button></TableCell> : null}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </DataState>
        </CardContent>
      </Card>
    </PageStack>
  );
}

function MonitorGroupCreateDrawer({ onCreated, onOpenChange, open }: { onCreated: () => Promise<void>; onOpenChange: (open: boolean) => void; open: boolean }) {
  const { t } = useI18n();
  async function handleCreated() {
    await onCreated();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("monitors.createGroup")}</SheetTitle>
          <SheetDescription>{t("monitors.createGroupDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          <MonitorGroupCreateForm onCreated={handleCreated} />
        </div>
      </SheetContent>
    </Sheet>
  );
}

function MonitorCreateDrawer({ groupOptions, onCreated, onOpenChange, open }: { groupOptions: ResourceOption[]; onCreated: () => Promise<void>; onOpenChange: (open: boolean) => void; open: boolean }) {
  const { t } = useI18n();
  async function handleCreated() {
    await onCreated();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("monitors.createMonitor")}</SheetTitle>
          <SheetDescription>{t("monitors.createMonitorDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          <MonitorCreateForm groupOptions={groupOptions} onCreated={handleCreated} />
        </div>
      </SheetContent>
    </Sheet>
  );
}

function MonitorGroupCreateForm({ onCreated }: { onCreated: () => Promise<void> }) {
  const { locale, t } = useI18n();
  async function createGroup(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    try {
      await controlPost<MonitorGroup>("/api/control/monitor-groups", {
        name: form.get("name"),
        description: form.get("description"),
      });
      formElement.reset();
      toast.success(t("monitors.groupCreated"));
      await onCreated();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={createGroup}>
      <FieldGroup>
        <TextField label={t("field.name")} name="name" placeholder={t("monitors.groupNamePlaceholder")} />
        <TextAreaField label={t("field.description")} name="description" placeholder={t("monitors.groupDescriptionPlaceholder")} />
      </FieldGroup>
      <Button type="submit"><PlusIcon data-icon="inline-start" />{t("targets.createGroup")}</Button>
    </form>
  );
}

function MonitorCreateForm({ groupOptions, onCreated }: { groupOptions: ResourceOption[]; onCreated: () => Promise<void> }) {
  const { locale, t } = useI18n();
  const [groupIDs, setGroupIDs] = useState<string[]>([]);

  async function createMonitor(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    try {
      await controlPost<Monitor>("/api/control/monitors", { name: form.get("name"), group_ids: groupIDs });
      formElement.reset();
      setGroupIDs([]);
      toast.success(t("monitors.monitorCreated"));
      await onCreated();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={createMonitor}>
      <FieldGroup>
        <TextField label={t("field.name")} name="name" placeholder="probe-a" />
        <ResourceMultiSelect label={t("monitors.monitorGroups")} onValueChange={setGroupIDs} options={groupOptions} values={groupIDs} />
      </FieldGroup>
      <Button disabled={groupIDs.length === 0} type="submit"><PlusIcon data-icon="inline-start" />{t("monitors.createMonitor")}</Button>
    </form>
  );
}
