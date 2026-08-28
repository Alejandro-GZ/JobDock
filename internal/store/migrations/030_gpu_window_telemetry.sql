ALTER TABLE gpus ADD COLUMN utilization_average_basis_points INTEGER;
ALTER TABLE gpus ADD COLUMN utilization_peak_basis_points INTEGER;
ALTER TABLE gpus ADD COLUMN utilization_sampled_at TEXT;
ALTER TABLE gpus ADD COLUMN utilization_window_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE gpus ADD COLUMN utilization_sample_count INTEGER NOT NULL DEFAULT 0;
