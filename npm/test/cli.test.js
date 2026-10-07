/**
 * 针对 CLI 调度脚本 (bin/cli.js) 的启动生命周期与进程拉起开展集成测试。
 *
 * @author Ateng
 * @since 2026-10-07
 */

import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const cliPath = path.resolve(__dirname, '../bin/cli.js');

describe('CLI 入口启动与进程管理 (cli.js)', () => {
  test('未找到可执行文件时输出中文排查错误并以状态码 1 退出', () => {
    const result = spawnSync(process.execPath, [cliPath], {
      env: {
        ...process.env,
        // 清理环境变量以防止命中本地覆盖
        MCP_SERVER_KUBERNETES_BINARY_PATH: '',
      },
      encoding: 'utf8',
    });

    assert.equal(result.status, 1);
    assert.match(
      result.stderr,
      /未找到适用于当前平台 .* 的原生可执行文件/
    );
  });

  test('通过环境变量注入 mock 二进制成功拉起子进程并透传参数与退出码', () => {
    // 使用当前 node 解释器模拟原生二进制
    const mockBinary = process.execPath;
    const result = spawnSync(
      process.execPath,
      [cliPath, '-e', 'console.log("MOCK_BINARY_SUCCESS"); process.exit(0);'],
      {
        env: {
          ...process.env,
          MCP_SERVER_KUBERNETES_BINARY_PATH: mockBinary,
        },
        encoding: 'utf8',
      }
    );

    assert.equal(result.status, 0);
    assert.match(result.stdout, /MOCK_BINARY_SUCCESS/);
  });

  test('正确透传非零退出码', () => {
    const mockBinary = process.execPath;
    const result = spawnSync(
      process.execPath,
      [cliPath, '-e', 'process.exit(42);'],
      {
        env: {
          ...process.env,
          MCP_SERVER_KUBERNETES_BINARY_PATH: mockBinary,
        },
        encoding: 'utf8',
      }
    );

    assert.equal(result.status, 42);
  });
});
