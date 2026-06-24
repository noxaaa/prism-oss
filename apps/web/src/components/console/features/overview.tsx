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
import { defaultConsoleRegistry } from "@/components/console/edition-registry";
import { hasAnyPermission } from "@/components/console/permissions";
import { useI18n } from "@/components/console/i18n";
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
  TableSkeleton,
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

export function AdminOverviewPage() {
  const { locale, t } = useI18n();
  const { session } = useConsoleSession();
  const hasMonitorsCapability = defaultConsoleRegistry.capabilities.includes("monitors");
  const canReadNodes = hasAnyPermission(session, ["nodes.read", "nodes.manage"]);
  const canReadMonitors = hasMonitorsCapability && hasAnyPermission(session, ["monitors.read", "monitors.manage"]);
  const canReadRules = hasAnyPermission(session, ["rules.read_all", "rules.manage_all", "rules.manage_own"]);
  const canReadTargets = hasAnyPermission(session, ["targets.read", "targets.manage"]);
  const nodes = useControlList<NodeResource>(canReadNodes ? "/api/control/nodes" : "");
  const monitors = useControlList<Monitor>(canReadMonitors ? "/api/control/monitors" : "");
  const rules = useControlList<Rule>(canReadRules ? "/api/control/rules" : "");
  const targets = useControlList<Target>(canReadTargets ? "/api/control/targets" : "");

  return (
    <PageStack>
      <SummaryGrid>
        <SummaryCard icon={<ServerIcon />} label={t("overview.nodes")} loading={nodes.loading} value={nodes.data.length} />
        {hasMonitorsCapability ? <SummaryCard icon={<RadarIcon />} label={t("overview.monitors")} loading={monitors.loading} value={monitors.data.length} /> : null}
        <SummaryCard icon={<RouteIcon />} label={t("overview.rules")} loading={rules.loading} value={rules.data.length} />
        <SummaryCard icon={<TargetIcon />} label={t("overview.targets")} loading={targets.loading} value={targets.data.length} />
      </SummaryGrid>
      <Card>
        <CardHeader>
          <CardTitle>{t("overview.runtimeStatus")}</CardTitle>
          <CardDescription>{session?.organization?.name ?? t("overview.currentOrganization")}</CardDescription>
        </CardHeader>
        <CardContent>
          <DataState loading={nodes.loading} loadingFallback={<TableSkeleton columns={5} rows={5} />} error={nodes.error}>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("overview.node")}</TableHead>
                  <TableHead>{t("overview.status")}</TableHead>
                  <TableHead>{t("overview.desired")}</TableHead>
                  <TableHead>{t("overview.applied")}</TableHead>
                  <TableHead>{t("overview.lastSeen")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {nodes.data.map((node) => (
                  <TableRow key={node.id}>
                    <TableCell>{node.name}</TableCell>
                    <TableCell><StatusBadge value={node.status} /></TableCell>
                    <TableCell>{node.desired_config_version}</TableCell>
                    <TableCell>{node.applied_config_version}</TableCell>
                    <TableCell>{shortDate(node.last_seen_at, locale)}</TableCell>
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
