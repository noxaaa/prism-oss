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
import { Alert, AlertDescription, AlertTitle } from "@noxaaa/prism-oss-web-core/ui/alert";
import { Badge } from "@noxaaa/prism-oss-web-core/ui/badge";
import { Button } from "@noxaaa/prism-oss-web-core/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@noxaaa/prism-oss-web-core/ui/card";
import { Checkbox } from "@noxaaa/prism-oss-web-core/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@noxaaa/prism-oss-web-core/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@noxaaa/prism-oss-web-core/ui/dropdown-menu";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@noxaaa/prism-oss-web-core/ui/empty";
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldSet, FieldLegend } from "@noxaaa/prism-oss-web-core/ui/field";
import { Input } from "@noxaaa/prism-oss-web-core/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@noxaaa/prism-oss-web-core/ui/select";
import { Separator } from "@noxaaa/prism-oss-web-core/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@noxaaa/prism-oss-web-core/ui/sheet";
import { Skeleton } from "@noxaaa/prism-oss-web-core/ui/skeleton";
import { Switch } from "@noxaaa/prism-oss-web-core/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@noxaaa/prism-oss-web-core/ui/table";
import { Textarea } from "@noxaaa/prism-oss-web-core/ui/textarea";
import { bytes, controlDelete, controlGet, controlPatch, controlPost, optionLabel, shortDate } from "@noxaaa/prism-oss-web-core/console/control-api";
import { defaultConsoleRegistry } from "@noxaaa/prism-oss-web-core/console/edition-registry";
import { hasAnyPermission } from "@noxaaa/prism-oss-web-core/console/permissions";
import { useI18n } from "@noxaaa/prism-oss-web-core/console/i18n";
import { ResourceMultiSelect, ResourceSelect } from "@noxaaa/prism-oss-web-core/console/resource-select";
import { useConsoleSession } from "@noxaaa/prism-oss-web-core/console/shell";
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
} from "@noxaaa/prism-oss-web-core/console/shared";
import { cn } from "@noxaaa/prism-oss-web-core/lib/utils";
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
} from "@noxaaa/prism-oss-web-core/console/types";

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
