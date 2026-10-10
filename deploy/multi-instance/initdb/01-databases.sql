-- 多实例部署样例：首次启动建共享库（协调器只自动开通租户库）。
CREATE DATABASE IF NOT EXISTS sdlc_platform CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;
CREATE DATABASE IF NOT EXISTS sdlc_control_plane CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;
