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
import { hasAnyPermission } from "@/components/console/permissions";
import { localizeControlError, useI18n } from "@/components/console/i18n";
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

export function UserUsagePage() {
  const { locale, t } = useI18n();
  const { session } = useConsoleSession();
  const canReadRules = hasAnyPermission(session, ["rules.read_own", "rules.manage_own", "rules.read_all", "rules.manage_all"]);
  const canReadTraffic = hasAnyPermission(session, ["traffic.read_own", "traffic.read_all"]);
  const canReadAllTraffic = hasAnyPermission(session, ["traffic.read_all"]);
  const rules = useControlList<Rule>(canReadRules ? "/api/control/rules" : "");
  const [trafficByRule, setTrafficByRule] = useState<Record<string, RuleTraffic>>({});
  const trafficReadableRules = useMemo(
    () => rules.data.filter((rule) => canReadTraffic && (canReadAllTraffic || rule.owner_user_id === session?.user.id)),
    [canReadAllTraffic, canReadTraffic, rules.data, session?.user.id],
  );

  async function loadTraffic() {
    if (!canReadTraffic || trafficReadableRules.length === 0) {
      setTrafficByRule({});
      return;
    }
    const pairs = await Promise.all(trafficReadableRules.map(async (rule) => {
      try {
        return [rule.id, await controlGet<RuleTraffic>(`/api/control/rules/${rule.id}/traffic`)] as const;
      } catch (error) {
        toast.error(localizeControlError(error, locale));
        return [rule.id, null] as const;
      }
    }));
    setTrafficByRule(Object.fromEntries(pairs.filter((pair): pair is readonly [string, RuleTraffic] => pair[1] !== null)));
  }

  useEffect(() => {
    const readableRuleIDs = new Set(trafficReadableRules.map((rule) => rule.id));
    setTrafficByRule((current) => Object.fromEntries(Object.entries(current).filter(([ruleID]) => readableRuleIDs.has(ruleID))));
    if (canReadTraffic && trafficReadableRules.length > 0) {
      void loadTraffic();
    }
  }, [canReadTraffic, trafficReadableRules]);

  const totals = Object.values(trafficByRule).reduce(
    (sum, item) => ({
      upload: sum.upload + item.upload_bytes,
      download: sum.download + item.download_bytes,
      tcp: sum.tcp + item.tcp_connections,
      udp: sum.udp + item.udp_packets,
    }),
    { upload: 0, download: 0, tcp: 0, udp: 0 },
  );

  return (
    <PageStack>
      <SummaryGrid>
        <SummaryCard icon={<RouteIcon />} label={t("overview.rules")} value={rules.data.length} />
        <SummaryCard icon={<UploadIcon />} label={t("usage.upload")} value={bytes(totals.upload)} />
        <SummaryCard icon={<DownloadIcon />} label={t("usage.download")} value={bytes(totals.download)} />
        <SummaryCard icon={<ActivityIcon />} label={t("usage.tcpConnections")} value={totals.tcp} />
      </SummaryGrid>
      <Card>
        <CardHeader>
          <CardTitle>{t("usage.usageByRule")}</CardTitle>
          <CardAction><Button disabled={!canReadRules || !canReadTraffic} onClick={loadTraffic} size="sm" type="button" variant="outline">{t("usage.refreshTraffic")}</Button></CardAction>
        </CardHeader>
        <CardContent>
          <DataState loading={rules.loading} error={rules.error}>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("usage.rule")}</TableHead>
                  <TableHead>{t("usage.upload")}</TableHead>
                  <TableHead>{t("usage.download")}</TableHead>
                  <TableHead>{t("usage.udpPackets")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {trafficReadableRules.map((rule) => (
                  <TableRow key={rule.id}>
                    <TableCell>{rule.name}</TableCell>
                    <TableCell>{bytes(trafficByRule[rule.id]?.upload_bytes)}</TableCell>
                    <TableCell>{bytes(trafficByRule[rule.id]?.download_bytes)}</TableCell>
                    <TableCell>{trafficByRule[rule.id]?.udp_packets ?? 0}</TableCell>
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
