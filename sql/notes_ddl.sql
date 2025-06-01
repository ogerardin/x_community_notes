CREATE TABLE IF NOT EXISTS note (
    noteid bigint NOT NULL,
    noteauthorparticipantid character varying(255),
    createdatmillis bigint,
    tweetid character varying(255),
    classification character varying(255),
    believable character varying(255),
    harmful character varying(255),
    validationdifficulty character varying(255),
    misleadingother integer NOT NULL,
    misleadingfactualerror integer NOT NULL,
    misleadingmanipulatedmedia integer NOT NULL,
    misleadingoutdatedinformation integer NOT NULL,
    misleadingmissingimportantcontext integer NOT NULL,
    misleadingunverifiedclaimasfact integer NOT NULL,
    misleadingsatire integer NOT NULL,
    notmisleadingother integer NOT NULL,
    notmisleadingfactuallycorrect integer NOT NULL,
    notmisleadingoutdatedbutnotwhenwritten integer NOT NULL,
    notmisleadingclearlysatire integer NOT NULL,
    notmisleadingpersonalopinion integer NOT NULL,
    trustworthysources integer NOT NULL,
    summary character varying(8192),
    ismedianote integer NOT NULL,

    summary_ts tsvector GENERATED ALWAYS AS (to_tsvector('english'::regconfig, (summary)::text)) STORED
);


ALTER TABLE ONLY public.note
    ADD CONSTRAINT note_pkey PRIMARY KEY (noteid);

CREATE INDEX idx3yl33mmhbcw582lic7c7fqqu4 ON public.note USING btree (createdatmillis);
CREATE INDEX idxovqwtw36x36lo9smq4lbxjcps ON public.note USING btree (noteauthorparticipantid);
CREATE INDEX idxu0f5st3d4b4c55eh9kqyd3yk ON public.note USING btree (tweetid);
CREATE INDEX ts_idx ON public.note USING gin (summary_ts);
