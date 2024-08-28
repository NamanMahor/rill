import {
  createRuntimeServiceGetResource,
  createRuntimeServiceListResources,
  getRuntimeServiceGetResourceQueryKey,
  getRuntimeServiceListResourcesQueryKey,
  RpcStatus,
  runtimeServiceGetResource,
  runtimeServiceListResources,
  V1GetResourceResponse,
  V1ListResourcesResponse,
  V1ReconcileStatus,
  type V1Resource,
} from "@rilldata/web-common/runtime-client";
import type { CreateQueryOptions, QueryClient } from "@tanstack/svelte-query";
import { ErrorType } from "../../runtime-client/http-client";

export enum ResourceKind {
  ProjectParser = "rill.runtime.v1.ProjectParser",
  Source = "rill.runtime.v1.Source",
  Connector = "rill.runtime.v1.Connector",
  Model = "rill.runtime.v1.Model",
  MetricsView = "rill.runtime.v1.MetricsView",
  Report = "rill.runtime.v1.Report",
  Alert = "rill.runtime.v1.Alert",
  Theme = "rill.runtime.v1.Theme",
  Component = "rill.runtime.v1.Component",
  Dashboard = "rill.runtime.v1.Dashboard",
  API = "rill.runtime.v1.API",
}

export type UserFacingResourceKinds = Exclude<
  ResourceKind,
  ResourceKind.ProjectParser
>;

export const SingletonProjectParserName = "parser";

// In the UI, we shouldn't show the `rill.runtime.v1` prefix
export function prettyResourceKind(kind: string) {
  return kind.replace(/^rill\.runtime\.v1\./, "");
}

export function useResource<T = V1Resource>(
  instanceId: string,
  name: string,
  kind: ResourceKind,
  queryOptions?: CreateQueryOptions<
    V1GetResourceResponse,
    ErrorType<RpcStatus>,
    T // T is the return type of the `select` function
  >,
) {
  const defaultQueryOptions: CreateQueryOptions<
    V1GetResourceResponse,
    ErrorType<RpcStatus>,
    T
  > = {
    select: (data) => data?.resource as T,
    enabled: !!instanceId && !!name && !!kind,
  };

  return createRuntimeServiceGetResource(
    instanceId,
    {
      "name.kind": kind,
      "name.name": name,
    },
    {
      query: {
        ...defaultQueryOptions,
        ...queryOptions,
      },
    },
  );
}

/**
 * `useResourceV2` is a more flexible version of `useResource` that accepts
 *  any `queryOptions`, not just `select` and `queryClient`.
 */
export function useResourceV2<T = V1Resource>(
  instanceId: string,
  name: string,
  kind: ResourceKind,
  queryOptions?: CreateQueryOptions<
    V1GetResourceResponse,
    ErrorType<RpcStatus>,
    T // T is the return type of the `select` function
  >,
) {
  const defaultQueryOptions: CreateQueryOptions<
    V1GetResourceResponse,
    ErrorType<RpcStatus>,
    T
  > = {
    select: (data) => data?.resource as T,
    enabled: !!instanceId && !!name && !!kind,
  };

  return createRuntimeServiceGetResource(
    instanceId,
    {
      "name.kind": kind,
      "name.name": name,
    },
    {
      query: {
        ...defaultQueryOptions,
        ...queryOptions,
      },
    },
  );
}

export function useProjectParser(queryClient: QueryClient, instanceId: string) {
  return useResource(
    instanceId,
    SingletonProjectParserName,
    ResourceKind.ProjectParser,
    {
      queryClient,
    },
  );
}

export function useFilteredResources<T = Array<V1Resource>>(
  instanceId: string,
  kind: ResourceKind,
  selector: (data: V1ListResourcesResponse) => T = (data) =>
    data.resources as T,
) {
  return createRuntimeServiceListResources(
    instanceId,
    {
      kind,
    },
    {
      query: {
        select: selector,
      },
    },
  );
}

/**
 * Fetches all resources and filters them client side.
 * This is to improve network requests since we need the full list all the time as well.
 */
export function useClientFilteredResources(
  instanceId: string,
  kind: ResourceKind,
  filter: (res: V1Resource) => boolean = () => true,
) {
  return createRuntimeServiceListResources(instanceId, undefined, {
    query: {
      select: (data) =>
        data.resources?.filter(
          (res) => res.meta?.name?.kind === kind && filter(res),
        ) ?? [],
    },
  });
}

export function resourceIsLoading(resource?: V1Resource) {
  return (
    !!resource &&
    resource.meta?.reconcileStatus !== V1ReconcileStatus.RECONCILE_STATUS_IDLE
  );
}

export async function fetchResource(
  queryClient: QueryClient,
  instanceId: string,
  name: string,
  kind: ResourceKind,
) {
  const resp = await queryClient.fetchQuery({
    queryKey: getRuntimeServiceGetResourceQueryKey(instanceId, {
      "name.name": name,
      "name.kind": kind,
    }),
    queryFn: () =>
      runtimeServiceGetResource(instanceId, {
        "name.name": name,
        "name.kind": kind,
      }),
  });
  return resp.resource;
}

export async function fetchResources(
  queryClient: QueryClient,
  instanceId: string,
) {
  const resp = await queryClient.fetchQuery({
    queryKey: getRuntimeServiceListResourcesQueryKey(instanceId),
    queryFn: () => runtimeServiceListResources(instanceId, {}),
  });
  return resp.resources ?? [];
}
