type AccessRow = {
    projectId: number;
    teamId: number;
    role: string;
    canOperate: boolean;
};
type ConnectorRow = {
    projectId: number;
    id: string;
    name: string;
    status: string;
    lastCheckedAt: unknown;
};
type ActionRow = {
    projectId: number;
    connectorId: string;
    action: string;
    flagEnabled: boolean;
    capabilityVerified: boolean;
};
type SyncRow = {
    projectId: number;
    connectorId: string;
    status: string;
    lastErrorCode: string | null;
    lastSucceededAt: unknown;
    nextAttemptAt: unknown;
};
type ResourceRow = {
    projectId: number;
    id: string;
    connectorId: string;
    kind: string;
    status: string;
    name: string | null;
    fileType: string | null;
    showOnMap: unknown;
    sizeBytes: unknown;
    modelType: unknown;
    modelStatus: unknown;
    reconstructionProgress: unknown;
    errorCode: unknown;
    zipStatus: unknown;
    zipProgress: unknown;
    resourceStatus: unknown;
    fileCount: unknown;
    assetId: string | null;
    assetKind: string | null;
    assetStatus: string | null;
    assetFailureCode: string | null;
    remoteUpdatedAt: unknown;
    lastSeenAt: unknown;
};
type JobRow = {
    projectId: number;
    id: string;
    connectorId: string;
    jobType: string;
    action: string;
    status: string;
    progress: unknown;
    stage: string | null;
    attemptCount: unknown;
    reconciliationCount: unknown;
    lastErrorCode: string | null;
    assetIds: unknown;
    createdAt: unknown;
    updatedAt: unknown;
};
export declare function presentFlightHubModels(projectId: number, access: AccessRow, connectors: ConnectorRow[], actions: ActionRow[], syncRows: SyncRow[], resources: ResourceRow[], jobs: JobRow[]): {
    projectId: number;
    source: string;
    access: {
        role: string;
        canOperate: boolean;
        mode: string;
    };
    connectors: {
        id: string;
        name: string;
        status: string;
        lastCheckedAt: string | null;
        actions: {
            action: string;
            available: boolean;
            flagEnabled: boolean;
            capabilityVerified: boolean;
        }[];
    }[];
    syncStates: {
        connectorId: string;
        status: string;
        lastErrorCode: string | null;
        lastSucceededAt: string | null;
        nextAttemptAt: string | null;
    }[];
    models: {
        id: string;
        connectorId: string;
        name: string;
        status: string;
        fileType: string | null;
        showOnMap: boolean;
        sizeBytes: number | null;
        assetId: number | null;
        assetStatus: string | null;
        assetFailureCode: string | null;
        remoteUpdatedAt: string | null;
        lastSeenAt: string | null;
        source: string;
    }[];
    resources: {
        id: string;
        connectorId: string;
        status: string;
        modelType: number | null;
        modelStatus: number | null;
        reconstructionProgress: number | null;
        errorCode: number | null;
        zipStatus: number | null;
        zipProgress: number | null;
        resourceStatus: number | null;
        fileCount: number | null;
        sizeBytes: number | null;
        assetId: number | null;
        assetKind: string | null;
        assetStatus: string | null;
        assetFailureCode: string | null;
        lastSeenAt: string | null;
        source: string;
    }[];
    jobs: {
        id: string;
        connectorId: string;
        jobType: string;
        action: string;
        status: string;
        progress: number | null;
        stage: string | null;
        attemptCount: number;
        reconciliationCount: number;
        lastErrorCode: string | null;
        assetIds: number[];
        createdAt: string | null;
        updatedAt: string | null;
    }[];
};
export {};

export type FlightHubModelsWorkspace = ReturnType<typeof presentFlightHubModels>;
