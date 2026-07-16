"use client";

import { Button, Empty, Flex, Result, Spin } from "antd";

export function PageLoading({ label = "加载中" }: { label?: string }) {
  return (
    <Flex align="center" className="page-feedback" gap={10} justify="center">
      <Spin size="small" />
      <span>{label}</span>
    </Flex>
  );
}

export function PageError({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <Result
      extra={onRetry ? <Button onClick={onRetry}>重试</Button> : undefined}
      status="error"
      subTitle={message}
      title="加载失败"
    />
  );
}

export function PageEmpty({ title }: { title: string }) {
  return <Empty className="page-feedback" description={title} image={Empty.PRESENTED_IMAGE_SIMPLE} />;
}
