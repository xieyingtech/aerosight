CREATE INDEX IF NOT EXISTS observations_pose_project_device_time_idx
    ON observations (project_id, pose_device_id, pose_captured_at DESC)
    WHERE pose_spatial_quality IS NOT NULL;

CREATE INDEX IF NOT EXISTS observations_pose_project_track_time_idx
    ON observations (project_id, pose_captured_at DESC)
    WHERE pose_spatial_quality IS NOT NULL AND pose_standard_position IS NOT NULL;
