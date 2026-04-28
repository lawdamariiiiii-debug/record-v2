-- Create recordings table for storing video recording metadata
-- This table stores all recording information including download links from Gofile and Filester

CREATE TABLE IF NOT EXISTS recordings (
    -- Primary key
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Recording identification
    site TEXT NOT NULL,                    -- Streaming site (chaturbate, stripchat, etc.)
    channel TEXT NOT NULL,                 -- Channel username
    timestamp TIMESTAMPTZ NOT NULL,        -- Recording start time (with timezone)
    date TEXT NOT NULL,                    -- Date in YYYY-MM-DD format for easy filtering
    
    -- Recording metadata
    duration_seconds INTEGER NOT NULL,     -- Recording duration in seconds
    file_size_bytes BIGINT NOT NULL,       -- File size in bytes
    quality TEXT NOT NULL,                 -- Quality string (e.g., "2160p60", "1080p60")
    
    -- Storage URLs
    gofile_url TEXT NOT NULL,              -- Download URL from Gofile
    filester_url TEXT NOT NULL,            -- Download URL from Filester
    filester_chunks TEXT[],                -- Array of URLs for split files (> 10 GB)
    
    -- Workflow tracking
    session_id TEXT NOT NULL,              -- Workflow run identifier
    matrix_job TEXT NOT NULL,              -- Matrix job identifier
    
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW()   -- Auto-generated timestamp
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_recordings_site_channel ON recordings(site, channel);
CREATE INDEX IF NOT EXISTS idx_recordings_date ON recordings(date);
CREATE INDEX IF NOT EXISTS idx_recordings_timestamp ON recordings(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_recordings_session_id ON recordings(session_id);
CREATE INDEX IF NOT EXISTS idx_recordings_channel ON recordings(channel);
CREATE INDEX IF NOT EXISTS idx_recordings_site ON recordings(site);

-- Create a composite index for site + channel + date queries
CREATE INDEX IF NOT EXISTS idx_recordings_site_channel_date ON recordings(site, channel, date);

-- Add comments for documentation
COMMENT ON TABLE recordings IS 'Stores metadata for all recorded livestream videos';
COMMENT ON COLUMN recordings.id IS 'Unique identifier for each recording';
COMMENT ON COLUMN recordings.site IS 'Streaming platform (chaturbate, stripchat, etc.)';
COMMENT ON COLUMN recordings.channel IS 'Channel username/identifier';
COMMENT ON COLUMN recordings.timestamp IS 'When the recording started (with timezone)';
COMMENT ON COLUMN recordings.date IS 'Recording date in YYYY-MM-DD format for easy filtering';
COMMENT ON COLUMN recordings.duration_seconds IS 'Total recording duration in seconds';
COMMENT ON COLUMN recordings.file_size_bytes IS 'File size in bytes';
COMMENT ON COLUMN recordings.quality IS 'Video quality (e.g., 2160p60, 1080p60)';
COMMENT ON COLUMN recordings.gofile_url IS 'Download link from Gofile storage';
COMMENT ON COLUMN recordings.filester_url IS 'Download link from Filester storage';
COMMENT ON COLUMN recordings.filester_chunks IS 'Array of chunk URLs for files split due to size (>10GB)';
COMMENT ON COLUMN recordings.session_id IS 'GitHub Actions workflow session identifier';
COMMENT ON COLUMN recordings.matrix_job IS 'GitHub Actions matrix job identifier';
COMMENT ON COLUMN recordings.created_at IS 'When this record was created in the database';

-- Enable Row Level Security (RLS)
ALTER TABLE recordings ENABLE ROW LEVEL SECURITY;

-- Create policy to allow public read access (adjust based on your security requirements)
CREATE POLICY "Allow public read access" ON recordings
    FOR SELECT
    USING (true);

-- Create policy to allow authenticated inserts (for the service role)
CREATE POLICY "Allow authenticated inserts" ON recordings
    FOR INSERT
    WITH CHECK (true);

-- Create policy to allow authenticated updates (for the service role)
CREATE POLICY "Allow authenticated updates" ON recordings
    FOR UPDATE
    USING (true);

-- Create policy to allow authenticated deletes (for the service role)
CREATE POLICY "Allow authenticated deletes" ON recordings
    FOR DELETE
    USING (true);
