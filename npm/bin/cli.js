#!/usr/bin/env node

/**
 * mcp-server-kubernetes CLI 包装层入口，负责调度平台原生二进制进程并透明转发标准输入输出。
 *
 * @author Ateng
 * @since 2026-10-07
 */

import { spawn } from 'node:child_process';
import { resolveBinaryPath } from '../lib/resolver.js';

try {
  const binaryPath = resolveBinaryPath();

  const child = spawn(binaryPath, process.argv.slice(2), {
    stdio: 'inherit',
    env: process.env,
  });

  child.on('error', (err) => {
    console.error(`无法启动原生二进制进程: ${err.message}`);
    process.exit(1);
  });

  child.on('close', (code, signal) => {
    if (code !== null) {
      process.exit(code);
    } else if (signal) {
      process.kill(process.pid, signal);
    }
  });

  // 转发常见中断与终止信号给子进程
  const forwardSignals = ['SIGINT', 'SIGTERM', 'SIGHUP'];
  for (const sig of forwardSignals) {
    process.on(sig, () => {
      if (child.pid && !child.killed) {
        child.kill(sig);
      }
    });
  }
} catch (err) {
  console.error(`mcp-server-kubernetes 启动失败: ${err.message}`);
  process.exit(1);
}
