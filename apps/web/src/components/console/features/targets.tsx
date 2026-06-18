"use client";

import {
  ActivityIcon,
  CopyIcon,
  DownloadIcon,
  EditIcon,
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
  Trash2Icon,
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

function targetGroupMembers(group: TargetGroup) {
  return Array.isArray(group.members) ? group.members : [];
}

export function TargetsPage({ mode }: { mode: "admin" | "user" }) {
  const { t } = useI18n();
  const { session } = useConsoleSession();
  const canManage = hasPermission(session, "targets.manage");
  const targets = useControlList<Target>("/api/control/targets");
  const targetGroups = useControlList<TargetGroup>("/api/control/target-groups");
  const targetGroupMembershipsLoaded = !targetGroups.loading && !targetGroups.error;
  const targetOptionsByID = useMemo(
    () => new Map(targets.data.map((target) => [target.id, { value: target.id, label: `${target.name} (${target.host}:${target.port})${target.enabled ? "" : ` ${t("targets.memberDisabled")}`}` }])),
    [targets.data, t],
  );
  const enabledTargetOptions = targets.data.filter((target) => target.enabled).map((target) => targetOptionsByID.get(target.id) ?? { value: target.id, label: `${target.name} (${target.host}:${target.port})` });
  const targetGroupOptions = targetGroups.data.map((group) => ({ value: group.id, label: group.name }));
  const targetNamesByID = useMemo(() => new Map(targets.data.map((target) => [target.id, target.name])), [targets.data]);
  const targetGroupLabelsByTargetID = useMemo(() => {
    const labels = new Map<string, string[]>();
    for (const group of targetGroups.data) {
      for (const member of targetGroupMembers(group)) {
        labels.set(member.target_id, [...(labels.get(member.target_id) ?? []), group.name]);
      }
    }
    return labels;
  }, [targetGroups.data]);
  const [targetCreateOpen, setTargetCreateOpen] = useState(false);
  const [targetGroupCreateOpen, setTargetGroupCreateOpen] = useState(false);
  const [editingTarget, setEditingTarget] = useState<Target | null>(null);
  const [deletingTarget, setDeletingTarget] = useState<Target | null>(null);
  const [editingTargetGroup, setEditingTargetGroup] = useState<TargetGroup | null>(null);
  const [deletingTargetGroup, setDeletingTargetGroup] = useState<TargetGroup | null>(null);
  const editingTargetGroupIDs = useMemo(
    () => (editingTarget ? targetGroupIDsForTarget(targetGroups.data, editingTarget.id) : []),
    [editingTarget, targetGroups.data],
  );

  async function refreshAll() {
    await Promise.all([targets.refresh(), targetGroups.refresh()]);
  }

  function targetOptionsForTargetGroup(group: TargetGroup) {
    const merged = new Map(enabledTargetOptions.map((option) => [option.value, option]));
    for (const member of targetGroupMembers(group)) {
      const option = targetOptionsByID.get(member.target_id);
      if (option && !merged.has(member.target_id)) {
        merged.set(member.target_id, option);
      }
    }
    return [...merged.values()];
  }

  return (
    <PageStack>
      {canManage ? (
        <TargetCreateDrawer
          onCreated={refreshAll}
          onOpenChange={setTargetCreateOpen}
          open={targetCreateOpen}
          targetGroupOptions={targetGroupOptions}
        />
      ) : null}
      {canManage ? (
        <TargetGroupCreateDrawer
          onCreated={refreshAll}
          onOpenChange={setTargetGroupCreateOpen}
          open={targetGroupCreateOpen}
          targetOptions={enabledTargetOptions}
        />
      ) : null}
      {canManage ? (
        <TargetEditDrawer
          onChanged={refreshAll}
          onOpenChange={(open) => { if (!open) setEditingTarget(null); }}
          target={editingTarget}
          targetGroupIDs={editingTargetGroupIDs}
          targetGroupMembershipsLoaded={targetGroupMembershipsLoaded}
          targetGroupOptions={targetGroupOptions}
        />
      ) : null}
      {canManage ? (
        <TargetDeleteDialog
          onChanged={refreshAll}
          onOpenChange={(open) => { if (!open) setDeletingTarget(null); }}
          target={deletingTarget}
        />
      ) : null}
      {canManage ? (
        <TargetGroupEditDrawer
          onChanged={refreshAll}
          onOpenChange={(open) => { if (!open) setEditingTargetGroup(null); }}
          targetGroup={editingTargetGroup}
          targetOptions={editingTargetGroup ? targetOptionsForTargetGroup(editingTargetGroup) : enabledTargetOptions}
        />
      ) : null}
      {canManage ? (
        <TargetGroupDeleteDialog
          onChanged={refreshAll}
          onOpenChange={(open) => { if (!open) setDeletingTargetGroup(null); }}
          targetGroup={deletingTargetGroup}
        />
      ) : null}
      <div className="flex flex-wrap justify-end gap-2">
        {canManage ? (
          <>
            <Button onClick={() => setTargetCreateOpen(true)} type="button">
              <PlusIcon data-icon="inline-start" />
              {t("targets.createTarget")}
            </Button>
            <Button onClick={() => setTargetGroupCreateOpen(true)} type="button" variant="outline">
              <PlusIcon data-icon="inline-start" />
              {t("targets.createTargetGroup")}
            </Button>
          </>
        ) : null}
      </div>
      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{t("targets.targets")}</CardTitle>
            <CardDescription>{mode === "user" ? t("targets.visibleDescription") : t("targets.upstreamDescription")}</CardDescription>
          </CardHeader>
          <CardContent>
            <DataState loading={targets.loading} error={targets.error}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("field.name")}</TableHead>
                    <TableHead>{t("targets.address")}</TableHead>
                    <TableHead>{t("targets.groups")}</TableHead>
                    <TableHead>{t("overview.status")}</TableHead>
                    {canManage ? <TableHead>{t("common.actions")}</TableHead> : null}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {targets.data.map((target) => (
                    <TableRow key={target.id}>
                      <TableCell>{target.name}</TableCell>
                      <TableCell>{target.host}:{target.port}</TableCell>
                      <TableCell>{targetGroupLabelsByTargetID.get(target.id)?.join(", ") || t("targets.noGroups")}</TableCell>
                      <TableCell><StatusBadge value={target.enabled ? "ENABLED" : "DISABLED"} /></TableCell>
                      {canManage ? (
                        <TableCell>
                          <TargetRowActions onDelete={() => setDeletingTarget(target)} onEdit={() => setEditingTarget(target)} />
                        </TableCell>
                      ) : null}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </DataState>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t("targets.targetGroups")}</CardTitle>
            <CardDescription>{t("targets.poolDescription")}</CardDescription>
          </CardHeader>
          <CardContent>
            <DataState loading={targetGroups.loading} error={targetGroups.error}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("field.name")}</TableHead>
                    <TableHead>{t("targets.scheduler")}</TableHead>
                    <TableHead>{t("targets.members")}</TableHead>
                    {canManage ? <TableHead>{t("common.actions")}</TableHead> : null}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {targetGroups.data.map((group) => (
                    <TableRow key={group.id}>
                      <TableCell>
                        <div className="flex flex-col gap-1">
                          <span>{group.name}</span>
                          {group.description ? <span className="text-xs text-muted-foreground">{group.description}</span> : null}
                        </div>
                      </TableCell>
                      <TableCell>{group.scheduler}</TableCell>
                      <TableCell>{targetGroupMemberSummary(group, targetNamesByID, t("targets.noTargets"), t("targets.unknownTarget"), t("targets.memberDisabled"))}</TableCell>
                      {canManage ? (
                        <TableCell>
                          <TargetRowActions onDelete={() => setDeletingTargetGroup(group)} onEdit={() => setEditingTargetGroup(group)} />
                        </TableCell>
                      ) : null}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </DataState>
          </CardContent>
        </Card>
      </div>
    </PageStack>
  );
}

function targetGroupIDsForTarget(groups: TargetGroup[], targetID: string): string[] {
  return groups.filter((group) => targetGroupMembers(group).some((member) => member.target_id === targetID)).map((group) => group.id);
}

function targetGroupMemberSummary(group: TargetGroup, targetNamesByID: Map<string, string>, emptyLabel: string, unknownLabel: string, disabledLabel: string): string {
  const members = targetGroupMembers(group);
  if (members.length === 0) {
    return emptyLabel;
  }
  return members.map((member) => `${targetNamesByID.get(member.target_id) ?? unknownLabel} p${member.priority}${member.enabled ? "" : ` ${disabledLabel}`}`).join(", ");
}

function TargetRowActions({ onDelete, onEdit }: { onDelete: () => void; onEdit: () => void }) {
  const { t } = useI18n();
  return (
    <div className="flex flex-wrap gap-2">
      <Button onClick={onEdit} size="sm" type="button" variant="outline">
        <EditIcon data-icon="inline-start" />
        {t("common.edit")}
      </Button>
      <Button onClick={onDelete} size="sm" type="button" variant="outline">
        <Trash2Icon data-icon="inline-start" />
        {t("common.delete")}
      </Button>
    </div>
  );
}

function TargetCreateDrawer({
  onCreated,
  onOpenChange,
  open,
  targetGroupOptions,
}: {
  onCreated: () => Promise<void>;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  targetGroupOptions: ResourceOption[];
}) {
  const { t } = useI18n();
  async function handleCreated() {
    await onCreated();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("targets.createTarget")}</SheetTitle>
          <SheetDescription>{t("targets.createTargetDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          <TargetCreateForm onCreated={handleCreated} targetGroupOptions={targetGroupOptions} />
        </div>
      </SheetContent>
    </Sheet>
  );
}

function TargetEditDrawer({
  onChanged,
  onOpenChange,
  target,
  targetGroupIDs,
  targetGroupMembershipsLoaded,
  targetGroupOptions,
}: {
  onChanged: () => Promise<void>;
  onOpenChange: (open: boolean) => void;
  target: Target | null;
  targetGroupIDs: string[];
  targetGroupMembershipsLoaded: boolean;
  targetGroupOptions: ResourceOption[];
}) {
  const { t } = useI18n();
  async function handleChanged() {
    await onChanged();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={Boolean(target)}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("targets.editTarget")}</SheetTitle>
          <SheetDescription>{t("targets.editTargetDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          {target ? <TargetEditForm key={target.id} onChanged={handleChanged} target={target} targetGroupIDs={targetGroupIDs} targetGroupMembershipsLoaded={targetGroupMembershipsLoaded} targetGroupOptions={targetGroupOptions} /> : null}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function TargetDeleteDialog({ onChanged, onOpenChange, target }: { onChanged: () => Promise<void>; onOpenChange: (open: boolean) => void; target: Target | null }) {
  const { locale, t } = useI18n();
  const [busy, setBusy] = useState(false);

  async function deleteTarget() {
    if (!target) {
      return;
    }
    setBusy(true);
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/targets/${target.id}`);
      toast.success(t("targets.targetDeleted"));
      await onChanged();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={Boolean(target)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("targets.deleteTarget")}</DialogTitle>
          <DialogDescription>{target ? t("targets.deleteTargetQuestion", { name: target.name }) : ""}</DialogDescription>
        </DialogHeader>
        <DialogFooter showCloseButton>
          <Button disabled={busy || !target} onClick={() => void deleteTarget()} type="button" variant="destructive">
            <Trash2Icon data-icon="inline-start" />
            {t("common.delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function TargetGroupCreateDrawer({
  onCreated,
  onOpenChange,
  open,
  targetOptions,
}: {
  onCreated: () => Promise<void>;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  targetOptions: ResourceOption[];
}) {
  const { t } = useI18n();
  async function handleCreated() {
    await onCreated();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("targets.createTargetGroup")}</SheetTitle>
          <SheetDescription>{t("targets.createTargetGroupDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          <TargetGroupCreateForm onCreated={handleCreated} targetOptions={targetOptions} />
        </div>
      </SheetContent>
    </Sheet>
  );
}

function TargetGroupEditDrawer({
  onChanged,
  onOpenChange,
  targetGroup,
  targetOptions,
}: {
  onChanged: () => Promise<void>;
  onOpenChange: (open: boolean) => void;
  targetGroup: TargetGroup | null;
  targetOptions: ResourceOption[];
}) {
  const { t } = useI18n();
  async function handleChanged() {
    await onChanged();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={Boolean(targetGroup)}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("targets.editTargetGroup")}</SheetTitle>
          <SheetDescription>{t("targets.editTargetGroupDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          {targetGroup ? <TargetGroupEditForm key={targetGroup.id} onChanged={handleChanged} targetGroup={targetGroup} targetOptions={targetOptions} /> : null}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function TargetGroupDeleteDialog({ onChanged, onOpenChange, targetGroup }: { onChanged: () => Promise<void>; onOpenChange: (open: boolean) => void; targetGroup: TargetGroup | null }) {
  const { locale, t } = useI18n();
  const [busy, setBusy] = useState(false);

  async function deleteTargetGroup() {
    if (!targetGroup) {
      return;
    }
    setBusy(true);
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/target-groups/${targetGroup.id}`);
      toast.success(t("targets.targetGroupDeleted"));
      await onChanged();
      onOpenChange(false);
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={Boolean(targetGroup)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("targets.deleteTargetGroup")}</DialogTitle>
          <DialogDescription>{targetGroup ? t("targets.deleteTargetGroupQuestion", { name: targetGroup.name }) : ""}</DialogDescription>
        </DialogHeader>
        <DialogFooter showCloseButton>
          <Button disabled={busy || !targetGroup} onClick={() => void deleteTargetGroup()} type="button" variant="destructive">
            <Trash2Icon data-icon="inline-start" />
            {t("common.delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function TargetCreateForm({ onCreated, targetGroupOptions }: { onCreated: () => Promise<void>; targetGroupOptions: ResourceOption[] }) {
  const { locale, t } = useI18n();
  const [targetEnabled, setTargetEnabled] = useState(true);
  const [targetGroupIDs, setTargetGroupIDs] = useState<string[]>([]);

  async function createTarget(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    try {
      await controlPost<Target>("/api/control/targets", {
        name: form.get("name"),
        host: form.get("host"),
        port: Number(form.get("port")),
        enabled: targetEnabled,
        target_group_ids: targetGroupIDs,
      });
      formElement.reset();
      setTargetEnabled(true);
      setTargetGroupIDs([]);
      toast.success(t("targets.targetCreated"));
      await onCreated();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={createTarget}>
      <FieldGroup>
        <TextField label={t("field.name")} name="name" placeholder="origin-a" />
        <TextField label={t("targets.host")} name="host" placeholder="203.0.113.10" />
        <TextField label={t("rules.port")} name="port" placeholder="443" type="number" />
        <ResourceMultiSelect label={t("targets.targetGroups")} onValueChange={setTargetGroupIDs} options={targetGroupOptions} values={targetGroupIDs} />
        <Field orientation="horizontal">
          <Switch checked={targetEnabled} id="target_enabled" onCheckedChange={setTargetEnabled} />
          <FieldLabel htmlFor="target_enabled">{t("common.enabled")}</FieldLabel>
        </Field>
      </FieldGroup>
      <Button type="submit"><PlusIcon data-icon="inline-start" />{t("targets.createTarget")}</Button>
    </form>
  );
}

function TargetEditForm({
  onChanged,
  target,
  targetGroupIDs: initialTargetGroupIDs,
  targetGroupMembershipsLoaded,
  targetGroupOptions,
}: {
  onChanged: () => Promise<void>;
  target: Target;
  targetGroupIDs: string[];
  targetGroupMembershipsLoaded: boolean;
  targetGroupOptions: ResourceOption[];
}) {
  const { locale, t } = useI18n();
  const [targetEnabled, setTargetEnabled] = useState(target.enabled);
  const [targetGroupIDs, setTargetGroupIDs] = useState<string[]>(initialTargetGroupIDs);

  useEffect(() => {
    if (targetGroupMembershipsLoaded) {
      setTargetGroupIDs(initialTargetGroupIDs);
    }
  }, [initialTargetGroupIDs, target.id, targetGroupMembershipsLoaded]);

  async function updateTarget(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!targetGroupMembershipsLoaded) {
      toast.error(t("targets.membershipLoading"));
      return;
    }
    const form = new FormData(event.currentTarget);
    try {
      await controlPatch<Target>(`/api/control/targets/${target.id}`, {
        name: form.get("name"),
        host: form.get("host"),
        port: Number(form.get("port")),
        enabled: targetEnabled,
        target_group_ids: targetGroupIDs,
      });
      toast.success(t("targets.targetUpdated"));
      await onChanged();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={updateTarget}>
      <FieldGroup>
        <TextField defaultValue={target.name} label={t("field.name")} name="name" placeholder="origin-a" />
        <TextField defaultValue={target.host} label={t("targets.host")} name="host" placeholder="203.0.113.10" />
        <TextField defaultValue={String(target.port)} label={t("rules.port")} name="port" placeholder="443" type="number" />
        {!targetGroupMembershipsLoaded ? (
          <Alert>
            <AlertTitle>{t("targets.targetGroups")}</AlertTitle>
            <AlertDescription>{t("targets.membershipLoading")}</AlertDescription>
          </Alert>
        ) : null}
        <ResourceMultiSelect label={t("targets.targetGroups")} onValueChange={setTargetGroupIDs} options={targetGroupOptions} values={targetGroupIDs} />
        <Field orientation="horizontal">
          <Switch checked={targetEnabled} id="edit_target_enabled" onCheckedChange={setTargetEnabled} />
          <FieldLabel htmlFor="edit_target_enabled">{t("common.enabled")}</FieldLabel>
        </Field>
      </FieldGroup>
      <Button disabled={!targetGroupMembershipsLoaded} type="submit"><EditIcon data-icon="inline-start" />{t("common.save")}</Button>
    </form>
  );
}

function TargetGroupCreateForm({ onCreated, targetOptions }: { onCreated: () => Promise<void>; targetOptions: ResourceOption[] }) {
  const { locale, t } = useI18n();
  const [targetIDs, setTargetIDs] = useState<string[]>([]);

  async function createTargetGroup(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const priority = Number(form.get("priority") || 10);
    try {
      await controlPost<TargetGroup>("/api/control/target-groups", {
        name: form.get("name"),
        description: form.get("description"),
        members: targetIDs.map((targetID) => ({ target_id: targetID, priority, enabled: true })),
      });
      formElement.reset();
      setTargetIDs([]);
      toast.success(t("targets.targetGroupCreated"));
      await onCreated();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={createTargetGroup}>
      <FieldGroup>
        <TextField label={t("field.name")} name="name" placeholder={t("targets.groupNamePlaceholder")} />
        <TextAreaField label={t("field.description")} name="description" placeholder={t("targets.groupDescriptionPlaceholder")} />
        <ResourceMultiSelect label={t("targets.targets")} onValueChange={setTargetIDs} options={targetOptions} values={targetIDs} />
        <TextField defaultValue="10" label={t("targets.defaultMemberPriority")} name="priority" placeholder="10" type="number" />
      </FieldGroup>
      <Button type="submit"><PlusIcon data-icon="inline-start" />{t("targets.createGroup")}</Button>
    </form>
  );
}

function TargetGroupEditForm({ onChanged, targetGroup, targetOptions }: { onChanged: () => Promise<void>; targetGroup: TargetGroup; targetOptions: ResourceOption[] }) {
  const { locale, t } = useI18n();
  const members = targetGroupMembers(targetGroup);
  const membersByTargetID = useMemo(() => new Map(members.map((member) => [member.target_id, member])), [members]);
  const [targetIDs, setTargetIDs] = useState<string[]>(members.map((member) => member.target_id));
  const defaultPriority = members[0]?.priority ?? 10;

  async function updateTargetGroup(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const priority = Number(form.get("priority") || defaultPriority);
    try {
      await controlPatch<TargetGroup>(`/api/control/target-groups/${targetGroup.id}`, {
        name: form.get("name"),
        description: form.get("description"),
        members: targetIDs.map((targetID) => {
          const existing = membersByTargetID.get(targetID);
          return { target_id: targetID, priority: existing?.priority ?? priority, enabled: existing?.enabled ?? true };
        }),
      });
      toast.success(t("targets.targetGroupUpdated"));
      await onChanged();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  return (
    <form className="flex flex-col gap-5" onSubmit={updateTargetGroup}>
      <FieldGroup>
        <TextField defaultValue={targetGroup.name} label={t("field.name")} name="name" placeholder={t("targets.groupNamePlaceholder")} />
        <Field>
          <FieldLabel htmlFor="edit-target-group-description">{t("field.description")}</FieldLabel>
          <Textarea defaultValue={targetGroup.description} id="edit-target-group-description" name="description" placeholder={t("targets.groupDescriptionPlaceholder")} />
        </Field>
        <ResourceMultiSelect label={t("targets.targets")} onValueChange={setTargetIDs} options={targetOptions} values={targetIDs} />
        <TextField defaultValue={String(defaultPriority)} label={t("targets.defaultMemberPriority")} name="priority" placeholder="10" type="number" />
      </FieldGroup>
      <Button type="submit"><EditIcon data-icon="inline-start" />{t("common.save")}</Button>
    </form>
  );
}
