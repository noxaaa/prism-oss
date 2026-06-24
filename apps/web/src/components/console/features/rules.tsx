"use client";

import {
  ActivityIcon,
  CopyIcon,
  DownloadIcon,
  EditIcon,
  MoreHorizontalIcon,
  NetworkIcon,
  PowerIcon,
  PowerOffIcon,
  PlusIcon,
  RadarIcon,
  RefreshCwIcon,
  RouteIcon,
  TargetIcon,
  Trash2Icon,
  UploadIcon,
} from "lucide-react";
import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { bytes, controlDelete, controlGet, controlPatch, controlPost, formatBitrateBps, shortDate } from "@/components/console/control-api";
import { localizeControlError, localizeEnum, localizeImportIssue, useI18n } from "@/components/console/i18n";
import { hasAnyPermission, hasPermission } from "@/components/console/permissions";
import { ResourceSelect } from "@/components/console/resource-select";
import { RuleDeploymentCell, RuleDeploymentSummary } from "@/components/console/rule-deployment-ui";
import { RuleMatchCell } from "@/components/console/rule-match-cell";
import { RuleUpstreamCell } from "@/components/console/rule-upstream-cell";
import { useConsoleSession } from "@/components/console/shell";
import {
  ControlledTextField,
  DataState,
  EnumSelect,
  PageStack,
  StatusBadge,
  SummaryCard,
  SummaryGrid,
  TableSkeleton,
  TextAreaField,
  TextField,
  copyText,
  ensureFirstValue,
  useControlList,
} from "@/components/console/shared";
import { cn } from "@/lib/utils";
import type { ResourceOption, Rule, RuleBatchResult, RuleDiagnostics, RuleExportPayload, RuleImportRequest, RuleImportResult, RuleTraffic, Target, TargetGroup } from "@/components/console/types";

const ruleProtocolOptions = [
  { value: "TCP", label: "TCP" },
  { value: "UDP", label: "UDP" },
  { value: "TCP_UDP", label: "TCP + UDP" },
];

const forwardingTypeOptions = [
  { value: "DIRECT", label: "Direct forwarding" },
];

const ruleFailurePolicyOptions = [
  { value: "KEEP_ENABLED", labelKey: "rules.failurePolicyKeepEnabled" },
  { value: "DISABLE_WHEN_ALL_NODES_FAILED", labelKey: "rules.failurePolicyDisableAllFailed" },
];

const proxyProtocolOptions = [
  { value: "NONE", label: "None" },
  { value: "V1", label: "Proxy Protocol v1" },
  { value: "V2", label: "Proxy Protocol v2" },
];

const ruleImportFormatOptions = [
  { value: "PORTABLE_EXPORT", label: "Portable export" },
  { value: "NYANPASS", label: "Nyanpass" },
];

function safeArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

function safeTags(rule: Rule): string[] {
  return safeArray(rule.tags);
}

export function RulesPage({ mode }: { mode: "admin" | "user" }) {
  const { locale, t } = useI18n();
  const { session } = useConsoleSession();
  const canManage = hasPermission(session, "rules.manage_all") || hasPermission(session, "rules.manage_own");
  const canReadTraffic = hasAnyPermission(session, ["traffic.read_own", "traffic.read_all"]);
  const canReadAllTraffic = hasPermission(session, "traffic.read_all");
  const canReadTargets = hasAnyPermission(session, ["targets.read", "targets.manage"]);
  const rules = useControlList<Rule>("/api/control/rules");
  const nodeGroupOptions = useControlList<ResourceOption>("/api/control/resource-options/node-groups?access=USE");
  const targets = useControlList<Target>(canReadTargets ? "/api/control/targets" : "");
  const targetGroups = useControlList<TargetGroup>(canReadTargets ? "/api/control/target-groups" : "");
  const targetOptions = useControlList<ResourceOption>(canManage ? "/api/control/resource-options/targets" : "");
  const targetGroupOptions = useControlList<ResourceOption>(canManage ? "/api/control/resource-options/target-groups" : "");
  const [trafficByRule, setTrafficByRule] = useState<Record<string, RuleTraffic>>({});
  const [createOpen, setCreateOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<Rule | null>(null);
  const [deleteRule, setDeleteRule] = useState<Rule | null>(null);
  const [diagnosticsRule, setDiagnosticsRule] = useState<Rule | null>(null);
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);
  const [batchBusy, setBatchBusy] = useState(false);
  const [batchResult, setBatchResult] = useState<RuleBatchResult | null>(null);
  const [selectedRuleIDs, setSelectedRuleIDs] = useState<string[]>([]);
  const [exportOpen, setExportOpen] = useState(false);
  const [exportLabel, setExportLabel] = useState("");
  const [exportPayload, setExportPayload] = useState<RuleExportPayload | null>(null);
  const selectedRuleIDSet = useMemo(() => new Set(selectedRuleIDs), [selectedRuleIDs]);
  const targetsByID = useMemo(() => new Map(targets.data.map((target) => [target.id, target])), [targets.data]);
  const targetGroupsByID = useMemo(() => new Map(targetGroups.data.map((group) => [group.id, group])), [targetGroups.data]);
  const targetOptionLabelsByID = useMemo(() => new Map(targetOptions.data.map((option) => [option.value, option.label])), [targetOptions.data]);
  const targetGroupOptionLabelsByID = useMemo(() => new Map(targetGroupOptions.data.map((option) => [option.value, option.label])), [targetGroupOptions.data]);
  const trafficReadableRules = useMemo(
    () => rules.data.filter((rule) => canReadTraffic && (canReadAllTraffic || rule.owner_user_id === session?.user.id)),
    [canReadAllTraffic, canReadTraffic, rules.data, session?.user.id],
  );

  useEffect(() => {
    const visibleRuleIDs = new Set(rules.data.map((rule) => rule.id));
    setSelectedRuleIDs((current) => {
      const next = current.filter((id) => visibleRuleIDs.has(id));
      return next.length === current.length ? current : next;
    });
  }, [rules.data]);

  useEffect(() => {
    let active = true;
    const readableRuleIDs = new Set(trafficReadableRules.map((rule) => rule.id));
    setTrafficByRule((current) => Object.fromEntries(Object.entries(current).filter(([ruleID]) => readableRuleIDs.has(ruleID))));
    if (trafficReadableRules.length === 0) {
      return () => {
        active = false;
      };
    }
    async function loadTraffic() {
      const pairs = await Promise.all(trafficReadableRules.map(async (rule) => {
        try {
          return [rule.id, await controlGet<RuleTraffic>(`/api/control/rules/${rule.id}/traffic`)] as const;
        } catch (error) {
          toast.error(localizeControlError(error, locale));
          return [rule.id, null] as const;
        }
      }));
      if (!active) {
        return;
      }
      setTrafficByRule(Object.fromEntries(pairs.filter((pair): pair is readonly [string, RuleTraffic] => pair[1] !== null)));
    }
    void loadTraffic();
    return () => {
      active = false;
    };
  }, [locale, trafficReadableRules]);

  async function ruleAction(ruleID: string, action: "enable" | "disable" | "copy") {
    try {
      await controlPost<Rule>(`/api/control/rules/${ruleID}/${action}`, {});
      if (action === "copy") {
        toast.success(t("rules.copyRequested"));
      } else {
        toast.success(action === "enable" ? t("rules.enableRequested") : t("rules.disableRequested"));
      }
      await rules.refresh();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function exportRules(ruleIDs: string[] | "all", label: string) {
    try {
      const query = ruleIDs === "all" ? "" : `?rule_ids=${encodeURIComponent(ruleIDs.join(","))}`;
      const payload = await controlGet<RuleExportPayload>(`/api/control/rules/export${query}`);
      setExportPayload(payload);
      setExportLabel(label);
      setExportOpen(true);
      toast.success(t("rules.exported", { label }));
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function batchRules(action: "ENABLE" | "DISABLE" | "DELETE") {
    if (selectedRuleIDs.length === 0) {
      return;
    }
    setBatchBusy(true);
    try {
      const result = await controlPost<RuleBatchResult>("/api/control/rules/batch", {
        action,
        rule_ids: selectedRuleIDs,
      });
      setBatchResult(result);
      if (result.failed > 0) {
        toast.error(t("rules.batchSummary", { succeeded: result.succeeded, failed: result.failed }));
      } else {
        toast.success(t("rules.batchUpdated", { count: result.succeeded }));
      }
      await rules.refresh();
      if (action === "DELETE") {
        setSelectedRuleIDs([]);
      }
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    } finally {
      setBatchBusy(false);
    }
  }

  async function deleteSelectedRule() {
    if (!deleteRule) {
      return;
    }
    try {
      await controlDelete<{ deleted: boolean }>(`/api/control/rules/${deleteRule.id}`);
      toast.success(t("rules.deleted"));
      setSelectedRuleIDs((current) => current.filter((id) => id !== deleteRule.id));
      setDeleteRule(null);
      await rules.refresh();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  async function confirmBatchDelete() {
    await batchRules("DELETE");
    setBatchDeleteOpen(false);
  }

  function toggleRuleSelected(ruleID: string, checked: boolean) {
    setSelectedRuleIDs((current) => {
      if (checked) {
        return current.includes(ruleID) ? current : [...current, ruleID];
      }
      return current.filter((id) => id !== ruleID);
    });
  }

  function toggleAllRules(checked: boolean) {
    setSelectedRuleIDs(checked ? rules.data.map((rule) => rule.id) : []);
  }

  const rulesTable = (
    <Card>
      <CardHeader>
        <CardTitle>{mode === "admin" ? t("rules.title") : t("rules.inventory")}</CardTitle>
        <CardDescription>{t("rules.description")}</CardDescription>
        <CardAction>
          <div className="flex flex-wrap gap-2">
            {canManage ? (
              <Button onClick={() => setCreateOpen(true)} size="sm" type="button">
                <PlusIcon data-icon="inline-start" />
                {t("rules.createRule")}
              </Button>
            ) : null}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button size="sm" type="button" variant="outline">
                  <MoreHorizontalIcon data-icon="inline-start" />
                  {t("common.options")}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-44">
                <DropdownMenuGroup>
                  {canManage ? (
                    <DropdownMenuItem onSelect={() => setImportOpen(true)}>
                      <UploadIcon />
                      {t("rules.importRules")}
                    </DropdownMenuItem>
                  ) : null}
                  {canManage ? (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem disabled={selectedRuleIDs.length === 0 || batchBusy} onSelect={() => void batchRules("ENABLE")}>
                        <PowerIcon />
                        {t("rules.enableSelected")}
                      </DropdownMenuItem>
                      <DropdownMenuItem disabled={selectedRuleIDs.length === 0 || batchBusy} onSelect={() => void batchRules("DISABLE")}>
                        <PowerOffIcon />
                        {t("rules.disableSelected")}
                      </DropdownMenuItem>
                      <DropdownMenuItem disabled={selectedRuleIDs.length === 0 || batchBusy} onSelect={() => setBatchDeleteOpen(true)}>
                        <Trash2Icon />
                        {t("rules.deleteSelected")}
                      </DropdownMenuItem>
                    </>
                  ) : null}
                  {canManage ? <DropdownMenuSeparator /> : null}
                  <DropdownMenuItem disabled={selectedRuleIDs.length === 0} onSelect={() => void exportRules(selectedRuleIDs, t("rules.selectedRules"))}>
                    <DownloadIcon />
                    {t("rules.exportSelected")}
                  </DropdownMenuItem>
                  <DropdownMenuItem onSelect={() => void exportRules("all", t("rules.allRules"))}>
                    <DownloadIcon />
                    {t("rules.exportAll")}
                  </DropdownMenuItem>
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
            <Button onClick={rules.refresh} size="sm" type="button" variant="outline"><RefreshCwIcon data-icon="inline-start" />{t("common.refresh")}</Button>
          </div>
        </CardAction>
      </CardHeader>
      <CardContent>
        {batchResult ? (
          <Alert className="mb-4">
            <AlertTitle>{t("rules.batchResult", { action: localizeEnum(batchResult.action, locale) })}</AlertTitle>
            <AlertDescription>
              {t("rules.batchSummary", { succeeded: batchResult.succeeded, failed: batchResult.failed })}
              {safeArray(batchResult.results).filter((item) => item.status === "FAILED").map((item) => ` ${item.rule_id}: ${item.error ? localizeControlError(item.error, locale) : localizeEnum("FAILED", locale)}`).join("")}
            </AlertDescription>
          </Alert>
        ) : null}
        <DataState loading={rules.loading} loadingFallback={<TableSkeleton columns={9} rows={6} />} error={rules.error}>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <Checkbox
                    aria-label={t("rules.selectAll")}
                    checked={rules.data.length > 0 && selectedRuleIDs.length === rules.data.length}
                    onCheckedChange={(checked) => toggleAllRules(checked === true)}
                  />
                </TableHead>
                <TableHead>{t("rules.name")}</TableHead>
                <TableHead>{t("rules.status")}</TableHead>
                <TableHead>{t("rules.deployment")}</TableHead>
                <TableHead>{t("rules.listener")}</TableHead>
                <TableHead>{t("rules.match")}</TableHead>
                <TableHead>{t("rules.upstream")}</TableHead>
                <TableHead>{t("rules.traffic")}</TableHead>
                <TableHead>{t("common.actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rules.data.map((rule) => (
                <TableRow key={rule.id}>
                  <TableCell>
                    <Checkbox
                      aria-label={t("rules.selectRule", { name: rule.name })}
                      checked={selectedRuleIDSet.has(rule.id)}
                      onCheckedChange={(checked) => toggleRuleSelected(rule.id, checked === true)}
                    />
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-1">
                      <span>{rule.name}</span>
                      <span className="text-xs text-muted-foreground">{safeTags(rule).join(", ") || t("common.noTags")}</span>
                    </div>
                  </TableCell>
                  <TableCell><StatusBadge value={rule.status} /></TableCell>
                  <TableCell><RuleDeploymentCell rule={rule} /></TableCell>
                  <TableCell>{localizeEnum(rule.protocol, locale)} {rule.listen_ip}:{rule.port}</TableCell>
                  <TableCell><RuleMatchCell rule={rule} /></TableCell>
                  <TableCell><RuleUpstreamCell rule={rule} targetGroupOptionLabelsByID={targetGroupOptionLabelsByID} targetGroupsByID={targetGroupsByID} targetOptionLabelsByID={targetOptionLabelsByID} targetsByID={targetsByID} /></TableCell>
                  <TableCell>{trafficByRule[rule.id] ? t("rules.trafficValue", { upload: bytes(trafficByRule[rule.id].upload_bytes), download: bytes(trafficByRule[rule.id].download_bytes) }) : t("common.notLoaded")}</TableCell>
                  <TableCell>
                    <RuleRowActions
                      canManage={canManage}
                      onCopy={() => void ruleAction(rule.id, "copy")}
                      onDelete={() => setDeleteRule(rule)}
                      onDiagnostics={() => setDiagnosticsRule(rule)}
                      onEdit={() => setEditingRule(rule)}
                      onExport={() => void exportRules([rule.id], rule.name)}
                      onToggle={() => void ruleAction(rule.id, rule.enabled ? "disable" : "enable")}
                      rule={rule}
                    />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DataState>
      </CardContent>
    </Card>
  );

  if (mode === "user") {
    return (
      <PageStack>
        {canManage ? <RuleCreateDrawer onCreated={rules.refresh} onOpenChange={setCreateOpen} open={createOpen} /> : null}
        {canManage ? <RuleEditDrawer onChanged={rules.refresh} onOpenChange={(open) => { if (!open) setEditingRule(null); }} rule={editingRule} /> : null}
        {canManage ? <RuleDeleteDialog onConfirm={deleteSelectedRule} onOpenChange={(open) => { if (!open) setDeleteRule(null); }} rule={deleteRule} /> : null}
        {canManage ? <RuleBatchDeleteDialog busy={batchBusy} count={selectedRuleIDs.length} onConfirm={confirmBatchDelete} onOpenChange={setBatchDeleteOpen} open={batchDeleteOpen} /> : null}
        {canManage ? <RuleImportDrawer onChanged={rules.refresh} onOpenChange={setImportOpen} open={importOpen} /> : null}
        <RuleDiagnosticsDialog onOpenChange={(open) => { if (!open) setDiagnosticsRule(null); }} rule={diagnosticsRule} />
        <RuleExportDialog exportLabel={exportLabel} exportPayload={exportPayload} onOpenChange={setExportOpen} open={exportOpen} />
        <SummaryGrid>
          <SummaryCard icon={<RouteIcon />} label={t("rules.activeRules")} loading={rules.loading} value={rules.data.filter((rule) => rule.enabled).length} />
          <SummaryCard icon={<ActivityIcon />} label={t("rules.totalRules")} loading={rules.loading} value={rules.data.length} />
          <SummaryCard icon={<NetworkIcon />} label={t("rules.availableNodeGroups")} loading={nodeGroupOptions.loading} value={nodeGroupOptions.data.filter((option) => !option.disabled).length} />
          <SummaryCard icon={<TargetIcon />} label={t("rules.scopes")} value={session?.resource_scopes?.length ?? 0} />
        </SummaryGrid>
        {rulesTable}
      </PageStack>
    );
  }

  return (
    <PageStack>
      {canManage ? <RuleCreateDrawer onCreated={rules.refresh} onOpenChange={setCreateOpen} open={createOpen} /> : null}
      {canManage ? <RuleEditDrawer onChanged={rules.refresh} onOpenChange={(open) => { if (!open) setEditingRule(null); }} rule={editingRule} /> : null}
      {canManage ? <RuleDeleteDialog onConfirm={deleteSelectedRule} onOpenChange={(open) => { if (!open) setDeleteRule(null); }} rule={deleteRule} /> : null}
      {canManage ? <RuleBatchDeleteDialog busy={batchBusy} count={selectedRuleIDs.length} onConfirm={confirmBatchDelete} onOpenChange={setBatchDeleteOpen} open={batchDeleteOpen} /> : null}
      {canManage ? <RuleImportDrawer onChanged={rules.refresh} onOpenChange={setImportOpen} open={importOpen} /> : null}
      <RuleDiagnosticsDialog onOpenChange={(open) => { if (!open) setDiagnosticsRule(null); }} rule={diagnosticsRule} />
      <RuleExportDialog exportLabel={exportLabel} exportPayload={exportPayload} onOpenChange={setExportOpen} open={exportOpen} />
      {rulesTable}
    </PageStack>
  );
}

function RuleRowActions({
  canManage,
  onCopy,
  onDelete,
  onDiagnostics,
  onEdit,
  onExport,
  onToggle,
  rule,
}: {
  canManage: boolean;
  onCopy: () => void;
  onDelete: () => void;
  onDiagnostics: () => void;
  onEdit: () => void;
  onExport: () => void;
  onToggle: () => void;
  rule: Rule;
}) {
  const { t } = useI18n();
  return (
    <div className="flex flex-wrap gap-2">
      {canManage ? (
        <Button onClick={onToggle} size="sm" type="button" variant="outline">
          {rule.enabled ? <PowerOffIcon data-icon="inline-start" /> : <PowerIcon data-icon="inline-start" />}
          {rule.enabled ? t("common.disable") : t("common.enable")}
        </Button>
      ) : null}
      <Button onClick={onExport} size="sm" type="button" variant="outline">
        <DownloadIcon data-icon="inline-start" />
        {t("common.export")}
      </Button>
      {canManage ? (
        <Button onClick={onEdit} size="sm" type="button" variant="outline">
          <EditIcon data-icon="inline-start" />
          {t("common.edit")}
        </Button>
      ) : null}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button aria-label={t("common.options")} size="icon" type="button" variant="outline">
            <MoreHorizontalIcon />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-40">
          <DropdownMenuGroup>
            <DropdownMenuItem onSelect={onDiagnostics}>
              <RadarIcon />
              {t("rules.diagnostics")}
            </DropdownMenuItem>
            {canManage ? (
              <>
                <DropdownMenuItem onSelect={onCopy}>
                  <CopyIcon />
                  {t("common.copy")}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem className="text-destructive focus:text-destructive" onSelect={onDelete}>
                  <Trash2Icon />
                  {t("common.delete")}
                </DropdownMenuItem>
              </>
            ) : null}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

function RuleDeleteDialog({ onConfirm, onOpenChange, rule }: { onConfirm: () => Promise<void>; onOpenChange: (open: boolean) => void; rule: Rule | null }) {
  const { t } = useI18n();
  return (
    <Dialog onOpenChange={onOpenChange} open={Boolean(rule)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("rules.deleteRule")}</DialogTitle>
          <DialogDescription>{rule ? t("rules.deleteQuestion", { name: rule.name }) : t("rules.deleteThisQuestion")}</DialogDescription>
        </DialogHeader>
        <DialogFooter showCloseButton>
          <Button onClick={() => void onConfirm()} type="button" variant="destructive">
            <Trash2Icon data-icon="inline-start" />
            {t("rules.deleteRule")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RuleDiagnosticsDialog({ onOpenChange, rule }: { onOpenChange: (open: boolean) => void; rule: Rule | null }) {
  const { locale, t } = useI18n();
  const [diagnostics, setDiagnostics] = useState<RuleDiagnostics | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    if (!rule) {
      setDiagnostics(null);
      setError("");
      return;
    }
    const selectedRule = rule;
    let active = true;
    async function loadDiagnostics() {
      setLoading(true);
      setError("");
      try {
        const rule = selectedRule;
        const result = await controlGet<RuleDiagnostics>(`/api/control/rules/${rule.id}/diagnostics`);
        if (active) {
          setDiagnostics(result);
        }
      } catch (loadError) {
        const message = localizeControlError(loadError, locale);
        if (active) {
          setError(message);
        }
        toast.error(message);
      } finally {
        if (active) {
          setLoading(false);
        }
      }
    }
    void loadDiagnostics();
    return () => {
      active = false;
    };
  }, [locale, refreshKey, rule]);

  const targets = safeArray(diagnostics?.targets);

  return (
    <Dialog onOpenChange={onOpenChange} open={Boolean(rule)}>
      <DialogContent className="max-h-[min(720px,calc(100vh-2rem))] overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t("rules.diagnostics")}</DialogTitle>
          <DialogDescription>{rule ? t("rules.diagnosticsDescription", { name: rule.name }) : ""}</DialogDescription>
        </DialogHeader>
        {error ? (
          <Alert variant="destructive">
            <AlertTitle>{t("rules.diagnosticsFailed")}</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <RuleDeploymentSummary rule={rule} />
        {diagnostics ? (
          <div className="space-y-4">
            <SummaryGrid>
              <SummaryCard icon={<ActivityIcon />} label={t("rules.currentBandwidth")} value={formatBitrateBps(diagnostics.bandwidth_bps)} />
              <SummaryCard icon={<UploadIcon />} label={t("usage.upload")} value={bytes(diagnostics.upload_bytes)} />
              <SummaryCard icon={<DownloadIcon />} label={t("usage.download")} value={bytes(diagnostics.download_bytes)} />
            </SummaryGrid>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("rules.target")}</TableHead>
                  <TableHead>{t("targets.address")}</TableHead>
                  <TableHead>{t("overview.status")}</TableHead>
                  <TableHead>{t("rules.latency")}</TableHead>
                  <TableHead>{t("rules.currentBandwidth")}</TableHead>
                  <TableHead>{t("rules.traffic")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {targets.map((target) => (
                  <TableRow key={target.target_id}>
                    <TableCell>{target.name || target.target_id}</TableCell>
                    <TableCell>{target.address}</TableCell>
                    <TableCell><StatusBadge value={target.status} /></TableCell>
                    <TableCell>{target.latency_ms == null ? t("common.notLoaded") : t("rules.latencyValue", { value: target.latency_ms })}</TableCell>
                    <TableCell>{target.bandwidth_bps == null ? t("common.notLoaded") : formatBitrateBps(target.bandwidth_bps)}</TableCell>
                    <TableCell>{t("rules.trafficValue", { upload: bytes(target.upload_bytes), download: bytes(target.download_bytes) })}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        ) : loading ? (
          <TableSkeleton columns={6} rows={3} />
        ) : null}
        <DialogFooter showCloseButton>
          <Button disabled={!rule || loading} onClick={() => setRefreshKey((value) => value + 1)} type="button" variant="outline">
            <RefreshCwIcon data-icon="inline-start" />
            {t("common.refresh")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RuleCreateDrawer({ onCreated, onOpenChange, open }: { onCreated: () => Promise<void>; onOpenChange: (open: boolean) => void; open: boolean }) {
  const { t } = useI18n();
  async function handleCreated() {
    await onCreated();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("rules.createRule")}</SheetTitle>
          <SheetDescription>{t("rules.createDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          <RuleMutationForm key="create-rule" onSaved={handleCreated} submitLabel={t("rules.createRule")} />
        </div>
      </SheetContent>
    </Sheet>
  );
}

function RuleEditDrawer({ onChanged, onOpenChange, rule }: { onChanged: () => Promise<void>; onOpenChange: (open: boolean) => void; rule: Rule | null }) {
  const { t } = useI18n();
  async function handleChanged() {
    await onChanged();
    onOpenChange(false);
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={Boolean(rule)}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("rules.editRule")}</SheetTitle>
          <SheetDescription>{t("rules.editDescription")}</SheetDescription>
        </SheetHeader>
        <div className="px-4 pb-4">
          {rule ? <RuleMutationForm key={rule.id} onSaved={handleChanged} rule={rule} submitLabel={t("rules.saveRule")} /> : null}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function RuleBatchDeleteDialog({ busy, count, onConfirm, onOpenChange, open }: { busy: boolean; count: number; onConfirm: () => Promise<void>; onOpenChange: (open: boolean) => void; open: boolean }) {
  const { t } = useI18n();
  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("rules.deleteSelected")}</DialogTitle>
          <DialogDescription>{t("rules.deleteSelectedQuestion", { count })}</DialogDescription>
        </DialogHeader>
        <DialogFooter showCloseButton>
          <Button disabled={busy || count === 0} onClick={() => void onConfirm()} type="button" variant="destructive">
            <Trash2Icon data-icon="inline-start" />
            {t("rules.deleteSelected")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RuleMutationForm({ onSaved, rule, submitLabel }: { onSaved: () => Promise<void>; rule?: Rule; submitLabel: string }) {
  const { locale, t } = useI18n();
  const [protocol, setProtocol] = useState(rule?.protocol ?? "TCP");
  const [port, setPort] = useState(String(rule?.port ?? 443));
  const [nodeGroupID, setNodeGroupID] = useState(rule?.node_group_id ?? "");
  const [listenIP, setListenIP] = useState(rule?.listen_ip ?? "");
  const [upstreamType, setUpstreamType] = useState(rule?.upstream.type ?? "TARGET_GROUP");
  const [targetID, setTargetID] = useState(rule?.upstream.target_id ?? "");
  const [targetGroupID, setTargetGroupID] = useState(rule?.upstream.target_group_id ?? "");
  const [matchType, setMatchType] = useState(rule?.match.type ?? "ANY_INBOUND");
  const [proxyIn, setProxyIn] = useState(rule?.proxy_protocol.in ?? "NONE");
  const [proxyOut, setProxyOut] = useState(rule?.proxy_protocol.out ?? "NONE");
  const [failurePolicy, setFailurePolicy] = useState(rule?.failure_policy ?? "KEEP_ENABLED");
  const [ruleEnabled, setRuleEnabled] = useState(rule?.enabled ?? false);
  const nodeGroups = useControlList<ResourceOption>("/api/control/resource-options/node-groups?access=USE");
  const targets = useControlList<ResourceOption>("/api/control/resource-options/targets");
  const targetGroups = useControlList<ResourceOption>("/api/control/resource-options/target-groups");
  const listenIPs = useControlList<ResourceOption>(
    nodeGroupID && Number(port) > 0 ? `/api/control/resource-options/node-group-listen-ips?node_group_id=${encodeURIComponent(nodeGroupID)}&protocol=${protocol}&port=${port}` : "",
  );
  const localizedProtocolOptions = ruleProtocolOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));
  const localizedForwardingTypeOptions = forwardingTypeOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));
  const localizedProxyProtocolOptions = proxyProtocolOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));
  const localizedFailurePolicyOptions = ruleFailurePolicyOptions.map((option) => ({ value: option.value, label: t(option.labelKey) }));
  const localizedMatchOptions = protocol !== "TCP"
    ? [{ value: "ANY_INBOUND", label: localizeEnum("ANY_INBOUND", locale) }]
    : [{ value: "ANY_INBOUND", label: localizeEnum("ANY_INBOUND", locale) }, { value: "TLS_SNI", label: localizeEnum("TLS_SNI", locale) }];
  const localizedUpstreamOptions = [{ value: "TARGET_GROUP", label: localizeEnum("TARGET_GROUP", locale) }, { value: "TARGET", label: localizeEnum("TARGET", locale) }];

  useEffect(() => {
    if (rule) {
      return;
    }
    ensureFirstValue(nodeGroups.data, nodeGroupID, setNodeGroupID);
    ensureFirstValue(targets.data, targetID, setTargetID);
    ensureFirstValue(targetGroups.data, targetGroupID, setTargetGroupID);
    ensureFirstValue(listenIPs.data, listenIP, setListenIP);
  }, [listenIPs.data, listenIP, nodeGroupID, nodeGroups.data, rule, targetGroupID, targetGroups.data, targetID, targets.data]);

  useEffect(() => {
    if (protocol !== "TCP" && matchType !== "ANY_INBOUND") {
      setMatchType("ANY_INBOUND");
    }
  }, [matchType, protocol]);

  useEffect(() => {
    if (protocol === "UDP") {
      setProxyIn("NONE");
      setProxyOut("NONE");
    }
  }, [protocol]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const body = {
      name: form.get("name"),
      tags: String(form.get("tags") ?? "").split(",").map((tag) => tag.trim()).filter(Boolean),
      node_group_id: nodeGroupID,
      listen_ip: listenIP,
      failure_policy: failurePolicy,
      forwarding_type: "DIRECT",
      protocol,
      port: Number(port),
      match: {
        type: matchType,
        sni_hostname: matchType === "TLS_SNI" ? form.get("sni_hostname") : "",
      },
      proxy_protocol: { in: proxyIn, out: proxyOut },
      upstream: upstreamType === "TARGET" ? { type: "TARGET", target_id: targetID } : { type: "TARGET_GROUP", target_group_id: targetGroupID },
      enabled: ruleEnabled,
    };
    try {
      if (rule) {
        await controlPatch<Rule>(`/api/control/rules/${rule.id}`, body);
        toast.success(t("rules.updated"));
      } else {
        await controlPost<Rule>("/api/control/rules", body);
        formElement.reset();
        setRuleEnabled(false);
        toast.success(t("rules.created"));
      }
      await onSaved();
    } catch (error) {
      toast.error(localizeControlError(error, locale));
    }
  }

  const canSubmit = nodeGroupID && listenIP && (upstreamType === "TARGET" ? targetID : targetGroupID);

  return (
    <form className="flex flex-col gap-5" onSubmit={submit}>
      <FieldGroup>
        <TextField defaultValue={rule?.name} label={t("field.name")} name="name" placeholder="customer-a-https" />
        <TextField defaultValue={safeArray(rule?.tags).join(",")} label={t("field.tags")} name="tags" placeholder="customer-a,https" required={false} />
        <ResourceSelect label={t("rules.nodeGroup")} onValueChange={setNodeGroupID} options={nodeGroups.data} value={nodeGroupID} />
        <EnumSelect label={t("rules.forwardingType")} onValueChange={() => undefined} options={localizedForwardingTypeOptions} value="DIRECT" />
        <div className="grid gap-3 md:grid-cols-2">
          <EnumSelect label={t("rules.protocol")} onValueChange={(value) => { setProtocol(value); setListenIP(""); }} options={localizedProtocolOptions} value={protocol} />
          <ControlledTextField label={t("rules.port")} onValueChange={(value) => { setPort(value); setListenIP(""); }} placeholder="443" type="number" value={port} />
        </div>
        <ResourceSelect label={t("rules.listenIP")} onValueChange={setListenIP} options={listenIPs.data} value={listenIP} />
        <EnumSelect label={t("rules.failurePolicy")} onValueChange={setFailurePolicy} options={localizedFailurePolicyOptions} value={failurePolicy} />
        <FieldDescription>{t("rules.failurePolicyDescription")}</FieldDescription>
        <div className={cn("grid gap-3", matchType === "TLS_SNI" ? "md:grid-cols-2" : "")}>
          <EnumSelect label={t("rules.matchType")} onValueChange={setMatchType} options={localizedMatchOptions} value={matchType} />
          {matchType === "TLS_SNI" ? <TextField defaultValue={rule?.match.sni_hostname ?? ""} label={t("rules.sniHostname")} name="sni_hostname" placeholder="app.customer.example" /> : null}
        </div>
        {protocol !== "UDP" ? (
          <div className="grid gap-3 md:grid-cols-2">
            <EnumSelect label={t("rules.proxyProtocolIn")} onValueChange={setProxyIn} options={localizedProxyProtocolOptions} value={proxyIn} />
            <EnumSelect label={t("rules.proxyProtocolOut")} onValueChange={setProxyOut} options={localizedProxyProtocolOptions} value={proxyOut} />
          </div>
        ) : null}
        <EnumSelect label={t("rules.upstreamType")} onValueChange={setUpstreamType} options={localizedUpstreamOptions} value={upstreamType} />
        {upstreamType === "TARGET" ? (
          <ResourceSelect label={t("rules.target")} onValueChange={setTargetID} options={targets.data} value={targetID} />
        ) : (
          <ResourceSelect label={t("rules.targetGroup")} onValueChange={setTargetGroupID} options={targetGroups.data} value={targetGroupID} />
        )}
        <Field orientation="horizontal">
          <Switch checked={ruleEnabled} id="rule_enabled" onCheckedChange={setRuleEnabled} />
          <FieldLabel htmlFor="rule_enabled">{t("common.enabled")}</FieldLabel>
        </Field>
      </FieldGroup>
      <Button disabled={!canSubmit} type="submit">
        {rule ? <EditIcon data-icon="inline-start" /> : <PlusIcon data-icon="inline-start" />}
        {submitLabel}
      </Button>
    </form>
  );
}

function RuleExportDialog({
  exportLabel,
  exportPayload,
  onOpenChange,
  open,
}: {
  exportLabel: string;
  exportPayload: RuleExportPayload | null;
  onOpenChange: (open: boolean) => void;
  open: boolean;
}) {
  const { t } = useI18n();
  const exportText = exportPayload ? JSON.stringify(exportPayload, null, 2) : "";

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-h-[min(720px,calc(100vh-2rem))] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{exportLabel || t("rules.exportTitle")}</DialogTitle>
          <DialogDescription>
            {t("rules.exportDescription")}
          </DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field>
            <FieldLabel>{t("rules.exportJson")}</FieldLabel>
            <Textarea readOnly rows={16} value={exportText} />
            <FieldDescription>
              {exportPayload ? t("rules.exportCount", { rules: exportPayload.rules.length, targets: exportPayload.targets.length, targetGroups: exportPayload.target_groups.length }) : t("rules.exportChooseAction")}
            </FieldDescription>
          </Field>
        </FieldGroup>
        <DialogFooter showCloseButton>
          <Button disabled={!exportText} onClick={() => copyText(exportText, t("common.copied"))} type="button">
            <CopyIcon data-icon="inline-start" />
            {t("rules.copyJson")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RuleImportDrawer({ onChanged, onOpenChange, open }: { onChanged: () => Promise<void>; onOpenChange: (open: boolean) => void; open: boolean }) {
  const { locale, t } = useI18n();
  const [importText, setImportText] = useState("");
  const [importFormat, setImportFormat] = useState("PORTABLE_EXPORT");
  const [importResult, setImportResult] = useState<RuleImportResult | null>(null);
  const [importError, setImportError] = useState("");
  const [nodeGroupID, setNodeGroupID] = useState("");
  const [listenIP, setListenIP] = useState("");
  const nodeGroups = useControlList<ResourceOption>("/api/control/resource-options/node-groups?access=USE");
  const listenIPs = useControlList<ResourceOption>(
    nodeGroupID ? `/api/control/resource-options/node-group-listen-ips?node_group_id=${encodeURIComponent(nodeGroupID)}` : "",
  );
  const localizedImportFormatOptions = ruleImportFormatOptions.map((option) => ({ ...option, label: localizeEnum(option.value, locale) }));

  useEffect(() => {
    ensureFirstValue(nodeGroups.data, nodeGroupID, setNodeGroupID);
    ensureFirstValue(listenIPs.data, listenIP, setListenIP);
  }, [listenIP, listenIPs.data, nodeGroupID, nodeGroups.data]);

  async function importRules(dryRun: boolean) {
    setImportError("");
    const body: RuleImportRequest = {
      entry: { node_group_id: nodeGroupID, listen_ip: listenIP },
      format: importFormat,
      source_text: importText,
    };
    try {
      const result = await controlPost<RuleImportResult>(`/api/control/rules/import?dry_run=${dryRun ? "true" : "false"}`, body);
      setImportResult(result);
      const errors = safeArray(result.errors);
      const warnings = safeArray(result.warnings);
      if (errors.length > 0) {
        toast.error(t("rules.importErrorsToast", { count: errors.length, first: localizeImportIssue(errors[0], locale) }));
      } else if (warnings.length > 0) {
        toast.warning(t("rules.importWarningsToast", { count: warnings.length, first: localizeImportIssue(warnings[0], locale) }));
      } else {
        toast.success(dryRun ? t("rules.dryRunCompleted") : t("rules.importCompleted"));
      }
      if (!dryRun) {
        await onChanged();
      }
    } catch (error) {
      const message = localizeControlError(error, locale);
      setImportError(message);
      toast.error(message);
    }
  }

  const importErrors = safeArray(importResult?.errors);
  const importWarnings = safeArray(importResult?.warnings);
  const canImport = Boolean(importText && nodeGroupID && listenIP);
  const importPlaceholder =
    importFormat === "NYANPASS"
      ? `{"dest":["103.219.194.40:28082"],"listen_port":443,"name":"hong-kong-test"}\n{"accept_proxy_protocol":1,"dest":["1.1.1.1:443"],"listen_port":43444,"name":"proxy-test","proxy_protocol":2}`
      : "{... rules.export.v1 ...}";
  const importDescription =
    importFormat === "NYANPASS"
      ? t("rules.importNyanpassDescription")
      : t("rules.importPortableDescription");

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle>{t("rules.importRules")}</SheetTitle>
          <SheetDescription>{t("rules.importDescription")}</SheetDescription>
        </SheetHeader>
        <form className="flex flex-col gap-5 px-4 pb-4" onSubmit={(event) => event.preventDefault()}>
          <FieldGroup>
            <ResourceSelect label={t("rules.nodeGroup")} onValueChange={(value) => { setNodeGroupID(value); setListenIP(""); }} options={nodeGroups.data} value={nodeGroupID} />
            <ResourceSelect label={t("rules.listenIP")} onValueChange={setListenIP} options={listenIPs.data} value={listenIP} />
            <EnumSelect label={t("field.format")} onValueChange={setImportFormat} options={localizedImportFormatOptions} value={importFormat} />
            <Field>
              <FieldLabel htmlFor="rules-import-json">{t("rules.importSource")}</FieldLabel>
              <Textarea
                id="rules-import-json"
                onChange={(event) => setImportText(event.currentTarget.value)}
                placeholder={importPlaceholder}
                rows={14}
                value={importText}
              />
              <FieldDescription>{importDescription}</FieldDescription>
            </Field>
          </FieldGroup>
          {importError ? (
            <Alert variant="destructive">
              <AlertTitle>{t("rules.importFailed")}</AlertTitle>
              <AlertDescription>{importError}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button disabled={!canImport} onClick={() => importRules(true)} type="button" variant="outline">{t("rules.dryRun")}</Button>
            <Button disabled={!canImport} onClick={() => importRules(false)} type="button"><UploadIcon data-icon="inline-start" />{t("common.import")}</Button>
          </div>
          {importResult ? (
            <Alert variant={importErrors.length > 0 ? "destructive" : "default"}>
              <AlertTitle>{t("rules.importResult", { mode: importResult.dry_run ? t("rules.dryRun") : t("common.import") })}</AlertTitle>
              <AlertDescription className="space-y-3 text-pretty">
                <div>{t("rules.importSummary", { created: importResult.created, skipped: importResult.skipped, errors: importErrors.length, warnings: importWarnings.length })}</div>
                {importErrors.length > 0 ? (
                  <div>
                    <div className="font-medium text-foreground">{t("rules.importErrors")}</div>
                    <ul className="mt-1 list-disc space-y-1 pl-5">
                      {importErrors.map((error, index) => <li key={`import-error-${index}`}>{localizeImportIssue(error, locale)}</li>)}
                    </ul>
                  </div>
                ) : null}
                {importWarnings.length > 0 ? (
                  <div>
                    <div className="font-medium text-foreground">{t("rules.importWarnings")}</div>
                    <ul className="mt-1 list-disc space-y-1 pl-5">
                      {importWarnings.map((warning, index) => <li key={`import-warning-${index}`}>{localizeImportIssue(warning, locale)}</li>)}
                    </ul>
                  </div>
                ) : null}
              </AlertDescription>
            </Alert>
          ) : null}
        </form>
      </SheetContent>
    </Sheet>
  );
}
