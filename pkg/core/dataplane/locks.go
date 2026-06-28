package dataplane

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/noxaaa/prism-oss/pkg/core/agent"
	"github.com/noxaaa/prism-oss/pkg/core/domain"
)

func (manager *Manager) ensureExternalListenerLocks(plan plan) error {
	grouped := externalListenerLockGroups(plan)
	if len(grouped) == 0 {
		return nil
	}
	lockDir := filepath.Join(manager.options.RunDir, "listeners")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return configApplyError(err.Error(), lockErrorDetails(grouped, "", err.Error())...)
	}
	var acquired []string
	err := withListenerDirectoryLock(lockDir, func() error {
		if err := manager.ensureNoOverlappingExternalListenerLocks(lockDir, grouped); err != nil {
			return err
		}
		keys := make([]listenerKey, 0, len(grouped))
		for key := range grouped {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i int, j int) bool {
			return listenerKeyString(keys[i]) < listenerKeyString(keys[j])
		})
		acquired = make([]string, 0, len(keys))
		for _, key := range keys {
			entries := grouped[key]
			path := filepath.Join(lockDir, sanitizeName(listenerKeyString(key))+".lock")
			if err := manager.writeListenerLock(path, key, entries); err != nil {
				manager.releaseListenerLockPaths(acquired)
				return err
			}
			acquired = append(acquired, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func withListenerDirectoryLock(lockDir string, fn func() error) error {
	lockPath := filepath.Join(lockDir, ".prism-listeners.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }()
	return fn()
}

func (manager *Manager) ensureNoOverlappingExternalListenerLocks(lockDir string, grouped map[listenerKey][]plannedRule) error {
	entries, err := os.ReadDir(lockDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock") {
			continue
		}
		path := filepath.Join(lockDir, entry.Name())
		dataBytes, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		data := string(dataBytes)
		existingKey, ok := listenerKeyFromLockData(data)
		if !ok {
			continue
		}
		owner := parseLockValue(data, "instance_id")
		pid := parseLockValue(data, "pid")
		for key, planned := range grouped {
			if !listenerKeysOverlap(existingKey, key) {
				continue
			}
			if owner == manager.options.InstanceID && manager.lockOwnedByCurrentProcessOrStale(data) {
				_ = os.Remove(path)
				break
			}
			if owner != "" && processMatchesLock(pid, parseLockValue(data, "process_start_time")) {
				return configApplyError("listener is owned by another Prism instance",
					listenerError(ErrorListenerOwnedByOtherPrismInstance, key, rulesFromPlanned(planned), planned[0].dataplane, owner, "listener is owned by another Prism instance"),
				)
			}
			if manager.lockMayProtectOtherNFTablesState(data) {
				return configApplyError("listener is owned by another Prism instance",
					listenerError(ErrorListenerOwnedByOtherPrismInstance, key, rulesFromPlanned(planned), planned[0].dataplane, owner, "listener is owned by another Prism instance"),
				)
			}
			_ = os.Remove(path)
			break
		}
	}
	return nil
}

func (manager *Manager) ensureNoLiveOwnedListenerLocksFromOtherProcess(plan plan) error {
	lockDir := filepath.Join(manager.options.RunDir, "listeners")
	grouped := externalListenerLockGroups(plan)
	return withExistingListenerDirectoryLock(lockDir, func() error {
		entries, err := os.ReadDir(lockDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock") {
				continue
			}
			path := filepath.Join(lockDir, entry.Name())
			dataBytes, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			data := string(dataBytes)
			if parseLockValue(data, "instance_id") != manager.options.InstanceID {
				continue
			}
			pid := parseLockValue(data, "pid")
			if strings.TrimSpace(pid) == strconv.Itoa(os.Getpid()) || !processMatchesLock(pid, parseLockValue(data, "process_start_time")) {
				continue
			}
			existingKey, ok := listenerKeyFromLockData(data)
			if !ok {
				return liveOwnedLockError(existingKey, nil, parseLockValue(data, "dataplane"), manager.options.InstanceID)
			}
			lockDataplane := parseLockValue(data, "dataplane")
			for key, planned := range grouped {
				if listenerKeysOverlap(existingKey, key) {
					return liveOwnedLockError(key, planned, planned[0].dataplane, manager.options.InstanceID)
				}
			}
			return liveOwnedLockError(existingKey, nil, lockDataplane, manager.options.InstanceID)
		}
		return nil
	})
}

func withExistingListenerDirectoryLock(lockDir string, fn func() error) error {
	info, err := os.Stat(lockDir)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return withListenerDirectoryLock(lockDir, fn)
}

func liveOwnedLockError(key listenerKey, planned []plannedRule, dataplane string, owner string) error {
	rules := rulesFromPlanned(planned)
	if len(rules) == 0 {
		rules = []agent.RuleConfig{{
			ID:        "existing-listener-lock",
			Protocol:  key.protocol,
			ListenIP:  key.listenIP,
			Port:      key.port,
			MatchType: string(domain.MatchTypeAnyInbound),
		}}
	}
	return configApplyError("listener is owned by another Prism process",
		listenerError(ErrorListenerOwnedByOtherPrismInstance, key, rules, dataplane, owner, "listener is owned by another Prism process"),
	)
}

func externalListenerLockGroups(plan plan) map[listenerKey][]plannedRule {
	grouped := map[listenerKey][]plannedRule{}
	for _, entry := range plan.planned {
		for _, key := range listenerKeysForRule(entry.rule) {
			grouped[key] = append(grouped[key], entry)
		}
	}
	return grouped
}

func (manager *Manager) removeStaleExternalListenerLocks(plan plan) error {
	grouped := externalListenerLockGroups(plan)
	lockDir := filepath.Join(manager.options.RunDir, "listeners")
	return manager.removeStaleListenerLocks(lockDir, grouped)
}

func (manager *Manager) removeStaleListenerLocks(lockDir string, grouped map[listenerKey][]plannedRule) error {
	desired := make(map[string]struct{}, len(grouped))
	for key := range grouped {
		desired[sanitizeName(listenerKeyString(key))+".lock"] = struct{}{}
	}
	entries, err := os.ReadDir(lockDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock") {
			continue
		}
		if _, ok := desired[entry.Name()]; ok {
			continue
		}
		path := filepath.Join(lockDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if parseLockValue(string(data), "instance_id") == manager.options.InstanceID && manager.lockOwnedByCurrentProcessOrStale(string(data)) {
			_ = os.Remove(path)
		}
	}
	return nil
}

func (manager *Manager) releaseExternalListenerLocks(plan plan) {
	lockDir := filepath.Join(manager.options.RunDir, "listeners")
	for _, entry := range plan.planned {
		for _, key := range listenerKeysForRule(entry.rule) {
			path := filepath.Join(lockDir, sanitizeName(listenerKeyString(key))+".lock")
			manager.releaseListenerLockPath(path)
		}
	}
}

func (manager *Manager) releaseListenerLockPaths(paths []string) {
	for _, path := range paths {
		manager.releaseListenerLockPath(path)
	}
}

func (manager *Manager) releaseListenerLockPath(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if parseLockValue(string(data), "instance_id") == manager.options.InstanceID && manager.lockOwnedByCurrentProcessOrStale(string(data)) {
		_ = os.Remove(path)
	}
}

func (manager *Manager) writeListenerLock(path string, key listenerKey, entries []plannedRule) error {
	content := fmt.Sprintf("instance_id=%s\npid=%d\nprocess_start_time=%s\nservice=%s\ndataplane=%s\nlisten_protocol=%s\nlisten_ip=%s\nlisten_port=%d\n", manager.options.InstanceID, os.Getpid(), currentProcessStartTime(), manager.options.ServiceName, entries[0].dataplane, key.protocol, key.listenIP, key.port)
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			if _, writeErr := file.WriteString(content); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return writeErr
			}
			return file.Close()
		}
		if !os.IsExist(err) {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		owner := parseLockValue(string(data), "instance_id")
		pid := parseLockValue(string(data), "pid")
		if owner != "" && processMatchesLock(pid, parseLockValue(string(data), "process_start_time")) && (owner != manager.options.InstanceID || strings.TrimSpace(pid) != strconv.Itoa(os.Getpid())) {
			return configApplyError("listener is owned by another Prism instance",
				listenerError(ErrorListenerOwnedByOtherPrismInstance, key, rulesFromPlanned(entries), entries[0].dataplane, owner, "listener is owned by another Prism instance"),
			)
		}
		if manager.lockMayProtectOtherNFTablesState(string(data)) {
			return configApplyError("listener is owned by another Prism instance",
				listenerError(ErrorListenerOwnedByOtherPrismInstance, key, rulesFromPlanned(entries), entries[0].dataplane, owner, "listener is owned by another Prism instance"),
			)
		}
		if owner == manager.options.InstanceID && strings.TrimSpace(pid) == strconv.Itoa(os.Getpid()) || !processMatchesLock(pid, parseLockValue(string(data), "process_start_time")) {
			_ = os.Remove(path)
			continue
		}
		return configApplyError("listener lock is invalid",
			listenerError(ErrorListenerOwnedByOtherPrismInstance, key, rulesFromPlanned(entries), entries[0].dataplane, owner, "listener lock is invalid"),
		)
	}
	return configApplyError("listener lock could not be acquired",
		listenerError(ErrorListenerOwnedByOtherPrismInstance, key, rulesFromPlanned(entries), entries[0].dataplane, "", "listener lock could not be acquired"),
	)
}

func (manager *Manager) lockOwnedByCurrentProcessOrStale(data string) bool {
	pid := parseLockValue(data, "pid")
	return strings.TrimSpace(pid) == strconv.Itoa(os.Getpid()) || !processMatchesLock(pid, parseLockValue(data, "process_start_time"))
}

func (manager *Manager) lockMayProtectOtherNFTablesState(data string) bool {
	owner := strings.TrimSpace(parseLockValue(data, "instance_id"))
	if owner == "" || owner == manager.options.InstanceID {
		return false
	}
	if strings.TrimSpace(parseLockValue(data, "dataplane")) != ModeNFTables {
		return false
	}
	nftPath := strings.TrimSpace(manager.options.NFTPath)
	if nftPath == "" {
		nftPath = "nft"
	}
	tableName := nftablesTableNameForInstance(owner)
	output, err := manager.options.CommandRunner.CombinedOutput(context.Background(), nftPath, "list", "table", "inet", tableName)
	if err == nil {
		return true
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		return true
	}
	return !nftTableMissing(message)
}

func lockErrorDetails(grouped map[listenerKey][]plannedRule, owner string, message string) []agent.ConfigApplyErrorDetail {
	details := make([]agent.ConfigApplyErrorDetail, 0, len(grouped))
	for key, entries := range grouped {
		details = append(details, listenerError(ErrorListenerOwnedByOtherPrismInstance, key, rulesFromPlanned(entries), entries[0].dataplane, owner, message))
	}
	return details
}

func rulesFromPlanned(entries []plannedRule) []agent.RuleConfig {
	rules := make([]agent.RuleConfig, 0, len(entries))
	for _, entry := range entries {
		rules = append(rules, entry.rule)
	}
	return rules
}

func parseLockValue(data string, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func listenerKeyFromLockData(data string) (listenerKey, bool) {
	protocol := strings.TrimSpace(parseLockValue(data, "listen_protocol"))
	listenIP := strings.TrimSpace(parseLockValue(data, "listen_ip"))
	portText := strings.TrimSpace(parseLockValue(data, "listen_port"))
	if protocol == "" || listenIP == "" || portText == "" {
		return listenerKey{}, false
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return listenerKey{}, false
	}
	return listenerKey{protocol: domain.Protocol(protocol), listenIP: listenIP, port: port}, true
}

func processAlive(pid string) bool {
	value, err := strconv.Atoi(strings.TrimSpace(pid))
	if err != nil || value <= 0 {
		return false
	}
	process, err := os.FindProcess(value)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func processMatchesLock(pid string, expectedStartTime string) bool {
	if !processAlive(pid) {
		return false
	}
	expectedStartTime = strings.TrimSpace(expectedStartTime)
	if expectedStartTime == "" {
		return processAlive(pid)
	}
	return processStartTime(pid) == expectedStartTime
}

func currentProcessStartTime() string {
	return processStartTime(strconv.Itoa(os.Getpid()))
}

func processStartTime(pid string) string {
	value, err := strconv.Atoi(strings.TrimSpace(pid))
	if err != nil || value <= 0 {
		return ""
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(value), "stat"))
	if err != nil {
		return ""
	}
	text := string(data)
	end := strings.LastIndex(text, ")")
	if end < 0 || end+2 >= len(text) {
		return ""
	}
	fields := strings.Fields(text[end+2:])
	if len(fields) < 20 {
		return ""
	}
	return fields[19]
}
