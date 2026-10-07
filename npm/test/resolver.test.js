/**
 * 针对 Node.js 包装层二进制路径解析器与平台矩阵映射开展单元测试。
 *
 * @author Ateng
 * @since 2026-10-07
 */

import { test, describe } from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import {
  SUPPORTED_MATRIX,
  getPlatformArchKey,
  getSubpackageName,
  resolveBinaryPath,
} from '../lib/resolver.js';

describe('平台与芯片架构矩阵映射', () => {
  test('支持的五大主流平台矩阵清单完整', () => {
    assert.equal(SUPPORTED_MATRIX.length, 5);
    assert.deepEqual(SUPPORTED_MATRIX, [
      'darwin-arm64',
      'darwin-x64',
      'linux-x64',
      'linux-arm64',
      'win32-x64',
    ]);
  });

  test('正确计算标准平台架构 Key', () => {
    assert.equal(getPlatformArchKey('darwin', 'arm64'), 'darwin-arm64');
    assert.equal(getPlatformArchKey('darwin', 'x64'), 'darwin-x64');
    assert.equal(getPlatformArchKey('linux', 'x64'), 'linux-x64');
    assert.equal(getPlatformArchKey('linux', 'arm64'), 'linux-arm64');
    assert.equal(getPlatformArchKey('win32', 'x64'), 'win32-x64');
  });

  test('不支持的平台或架构抛出明确异常', () => {
    assert.throws(
      () => getPlatformArchKey('freebsd', 'x64'),
      /不支持的操作系统平台或芯片架构: freebsd-x64/
    );
    assert.throws(
      () => getPlatformArchKey('linux', 'ia32'),
      /不支持的操作系统平台或芯片架构: linux-ia32/
    );
  });

  test('获取各平台对应的子包名称', () => {
    assert.equal(
      getSubpackageName('darwin-arm64'),
      '@atengk/mcp-server-kubernetes-darwin-arm64'
    );
    assert.equal(
      getSubpackageName('win32-x64'),
      '@atengk/mcp-server-kubernetes-win32-x64'
    );
  });
});

describe('二进制路径解析 (resolveBinaryPath)', () => {
  test('通过环境变量 MCP_SERVER_KUBERNETES_BINARY_PATH 优先覆盖 (存在时有效)', () => {
    const customPath = path.resolve('/custom/path/to/mcp-server');
    const resolved = resolveBinaryPath({
      env: { MCP_SERVER_KUBERNETES_BINARY_PATH: customPath },
      platform: 'linux',
      arch: 'x64',
      fileExists: () => true,
    });
    assert.equal(resolved, customPath);
  });

  test('环境变量指定的不存在二进制路径抛出明确异常', () => {
    const nonExistentPath = path.resolve('/non/existent/mcp-server');
    assert.throws(
      () =>
        resolveBinaryPath({
          env: { MCP_SERVER_KUBERNETES_BINARY_PATH: nonExistentPath },
          platform: 'linux',
          arch: 'x64',
          fileExists: () => false,
        }),
      /环境变量 MCP_SERVER_KUBERNETES_BINARY_PATH 指定的可执行文件不存在/
    );
  });

  test('在未找到子包或二进制文件时抛出友好的中文排查错误', () => {
    assert.throws(
      () =>
        resolveBinaryPath({
          env: {},
          platform: 'linux',
          arch: 'x64',
          subpackageResolver: () => {
            throw new Error('Cannot find module');
          },
        }),
      /未找到适用于当前平台 \(linux-x64\) 的原生可执行文件/
    );
  });

  test('Windows 平台返回 .exe 后缀的可执行文件路径', () => {
    const fakePkgDir = path.resolve('node_modules/@atengk/mcp-server-kubernetes-win32-x64');
    const resolved = resolveBinaryPath({
      env: {},
      platform: 'win32',
      arch: 'x64',
      subpackageResolver: () => path.join(fakePkgDir, 'package.json'),
      fileExists: () => true,
    });
    assert.equal(
      resolved,
      path.join(fakePkgDir, 'bin', 'mcp-server-kubernetes.exe')
    );
  });

  test('Unix 平台返回无后缀的可执行文件路径', () => {
    const fakePkgDir = path.resolve('node_modules/@atengk/mcp-server-kubernetes-darwin-arm64');
    const resolved = resolveBinaryPath({
      env: {},
      platform: 'darwin',
      arch: 'arm64',
      subpackageResolver: () => path.join(fakePkgDir, 'package.json'),
      fileExists: () => true,
    });
    assert.equal(
      resolved,
      path.join(fakePkgDir, 'bin', 'mcp-server-kubernetes')
    );
  });
});
