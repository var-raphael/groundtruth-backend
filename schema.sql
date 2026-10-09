--
-- PostgreSQL database dump
--

\restrict yv7kiqPRB7OZkKkGzZLaxhBB3gYVGVqLeuyVomjSKqRnjoAknl39imK1DUQwC3y

-- Dumped from database version 18.2
-- Dumped by pg_dump version 18.2

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: candidate_evidence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.candidate_evidence (
    candidate_id uuid NOT NULL,
    top_repos jsonb NOT NULL,
    contributions jsonb NOT NULL,
    contributions_error text,
    extracted_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: candidate_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.candidate_identities (
    auth_user_id uuid NOT NULL,
    github_id bigint NOT NULL,
    github_username text NOT NULL,
    github_token text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: candidate_reports; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.candidate_reports (
    candidate_id uuid NOT NULL,
    job_id uuid NOT NULL,
    score numeric(4,2) NOT NULL,
    stack_match text NOT NULL,
    has_trust_flag boolean DEFAULT false NOT NULL,
    report jsonb NOT NULL,
    generated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: candidates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.candidates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    job_id uuid NOT NULL,
    full_name text NOT NULL,
    email text NOT NULL,
    country text DEFAULT ''::text NOT NULL,
    timezone text DEFAULT ''::text NOT NULL,
    years_experience integer DEFAULT 0 NOT NULL,
    github_id bigint NOT NULL,
    github_username text NOT NULL,
    github_token text,
    linkedin text,
    x text,
    portfolio text,
    status text DEFAULT 'queued'::text NOT NULL,
    applied_at timestamp with time zone DEFAULT now() NOT NULL,
    city text NOT NULL,
    status_updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT status_valid CHECK ((status = ANY (ARRAY['queued'::text, 'extracting'::text, 'extracted'::text, 'scoring'::text, 'scored'::text, 'failed'::text, 'unscanned'::text])))
);


--
-- Name: card_payments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.card_payments (
    id bigint NOT NULL,
    recruiter_id text NOT NULL,
    brand text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: card_payments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.card_payments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: card_payments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.card_payments_id_seq OWNED BY public.card_payments.id;


--
-- Name: job_delete_confirmations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.job_delete_confirmations (
    token uuid DEFAULT gen_random_uuid() NOT NULL,
    job_id uuid NOT NULL,
    downloaded boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone DEFAULT (now() + '01:00:00'::interval) NOT NULL
);


--
-- Name: job_share_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.job_share_links (
    token uuid DEFAULT gen_random_uuid() NOT NULL,
    job_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_at timestamp with time zone
);


--
-- Name: jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    recruiter_id uuid NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    stack text[] DEFAULT '{}'::text[] NOT NULL,
    location_mode text DEFAULT 'anywhere'::text NOT NULL,
    location_countries text[] DEFAULT '{}'::text[] NOT NULL,
    min_years_experience integer DEFAULT 0 NOT NULL,
    candidate_limit integer DEFAULT 50 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    min_overlap_hours integer DEFAULT 0 NOT NULL,
    timezones text[] NOT NULL,
    showcase boolean DEFAULT false NOT NULL,
    CONSTRAINT location_mode_valid CHECK ((location_mode = ANY (ARRAY['anywhere'::text, 'country'::text, 'onsite'::text])))
);


--
-- Name: outreach_drafts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.outreach_drafts (
    candidate_id uuid NOT NULL,
    job_id uuid NOT NULL,
    subject text NOT NULL,
    body text NOT NULL,
    edited boolean DEFAULT false NOT NULL,
    generated_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: payment_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payment_events (
    event_hash text NOT NULL,
    event text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: recruiters; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.recruiters (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    google_id text NOT NULL,
    email text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    plan text DEFAULT 'free'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    paystack_customer_code text,
    paystack_subscription_code text,
    paystack_email_token text,
    plan_expires_at timestamp with time zone
);


--
-- Name: share_link_outreach_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.share_link_outreach_events (
    id bigint NOT NULL,
    share_token uuid NOT NULL,
    candidate_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: share_link_outreach_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.share_link_outreach_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: share_link_outreach_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.share_link_outreach_events_id_seq OWNED BY public.share_link_outreach_events.id;


--
-- Name: usage_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.usage_events (
    id bigint NOT NULL,
    recruiter_id uuid NOT NULL,
    candidate_id uuid NOT NULL,
    action text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT usage_action_valid CHECK ((action = ANY (ARRAY['outreach'::text, 'rescan'::text])))
);


--
-- Name: usage_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.usage_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: usage_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.usage_events_id_seq OWNED BY public.usage_events.id;


--
-- Name: card_payments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_payments ALTER COLUMN id SET DEFAULT nextval('public.card_payments_id_seq'::regclass);


--
-- Name: share_link_outreach_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.share_link_outreach_events ALTER COLUMN id SET DEFAULT nextval('public.share_link_outreach_events_id_seq'::regclass);


--
-- Name: usage_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usage_events ALTER COLUMN id SET DEFAULT nextval('public.usage_events_id_seq'::regclass);


--
-- Name: candidate_evidence candidate_evidence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.candidate_evidence
    ADD CONSTRAINT candidate_evidence_pkey PRIMARY KEY (candidate_id);


--
-- Name: candidate_identities candidate_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.candidate_identities
    ADD CONSTRAINT candidate_identities_pkey PRIMARY KEY (auth_user_id);


--
-- Name: candidate_reports candidate_reports_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.candidate_reports
    ADD CONSTRAINT candidate_reports_pkey PRIMARY KEY (candidate_id, job_id);


--
-- Name: candidates candidates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.candidates
    ADD CONSTRAINT candidates_pkey PRIMARY KEY (id);


--
-- Name: card_payments card_payments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_payments
    ADD CONSTRAINT card_payments_pkey PRIMARY KEY (id);


--
-- Name: job_delete_confirmations job_delete_confirmations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.job_delete_confirmations
    ADD CONSTRAINT job_delete_confirmations_pkey PRIMARY KEY (token);


--
-- Name: job_share_links job_share_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.job_share_links
    ADD CONSTRAINT job_share_links_pkey PRIMARY KEY (token);


--
-- Name: jobs jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.jobs
    ADD CONSTRAINT jobs_pkey PRIMARY KEY (id);


--
-- Name: outreach_drafts outreach_drafts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outreach_drafts
    ADD CONSTRAINT outreach_drafts_pkey PRIMARY KEY (candidate_id);


--
-- Name: payment_events payment_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_events
    ADD CONSTRAINT payment_events_pkey PRIMARY KEY (event_hash);


--
-- Name: recruiters recruiters_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recruiters
    ADD CONSTRAINT recruiters_email_key UNIQUE (email);


--
-- Name: recruiters recruiters_google_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recruiters
    ADD CONSTRAINT recruiters_google_id_key UNIQUE (google_id);


--
-- Name: recruiters recruiters_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recruiters
    ADD CONSTRAINT recruiters_pkey PRIMARY KEY (id);


--
-- Name: share_link_outreach_events share_link_outreach_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.share_link_outreach_events
    ADD CONSTRAINT share_link_outreach_events_pkey PRIMARY KEY (id);


--
-- Name: usage_events usage_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usage_events
    ADD CONSTRAINT usage_events_pkey PRIMARY KEY (id);


--
-- Name: card_payments_brand_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX card_payments_brand_idx ON public.card_payments USING btree (brand);


--
-- Name: idx_candidate_identities_github; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_candidate_identities_github ON public.candidate_identities USING btree (github_id);


--
-- Name: idx_candidate_reports_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_candidate_reports_job_id ON public.candidate_reports USING btree (job_id);


--
-- Name: idx_candidate_reports_score; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_candidate_reports_score ON public.candidate_reports USING btree (job_id, score DESC);


--
-- Name: idx_candidates_job_github; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_candidates_job_github ON public.candidates USING btree (job_id, github_id);


--
-- Name: idx_candidates_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_candidates_job_id ON public.candidates USING btree (job_id);


--
-- Name: idx_candidates_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_candidates_status ON public.candidates USING btree (status);


--
-- Name: idx_job_delete_confirmations_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_job_delete_confirmations_job_id ON public.job_delete_confirmations USING btree (job_id);


--
-- Name: idx_job_share_links_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_job_share_links_job_id ON public.job_share_links USING btree (job_id);


--
-- Name: idx_jobs_recruiter_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_jobs_recruiter_id ON public.jobs USING btree (recruiter_id);


--
-- Name: idx_jobs_showcase; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_jobs_showcase ON public.jobs USING btree (showcase) WHERE showcase;


--
-- Name: idx_outreach_drafts_job_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_outreach_drafts_job_id ON public.outreach_drafts USING btree (job_id);


--
-- Name: idx_recruiters_paystack_customer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_recruiters_paystack_customer ON public.recruiters USING btree (paystack_customer_code);


--
-- Name: idx_recruiters_paystack_subscription; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_recruiters_paystack_subscription ON public.recruiters USING btree (paystack_subscription_code);


--
-- Name: idx_share_link_outreach_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_share_link_outreach_token ON public.share_link_outreach_events USING btree (share_token);


--
-- Name: idx_usage_candidate_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_usage_candidate_time ON public.usage_events USING btree (candidate_id, created_at DESC);


--
-- Name: idx_usage_recruiter_action; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_usage_recruiter_action ON public.usage_events USING btree (recruiter_id, action, candidate_id);


--
-- Name: candidate_evidence candidate_evidence_candidate_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.candidate_evidence
    ADD CONSTRAINT candidate_evidence_candidate_id_fkey FOREIGN KEY (candidate_id) REFERENCES public.candidates(id) ON DELETE CASCADE;


--
-- Name: candidate_reports candidate_reports_candidate_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.candidate_reports
    ADD CONSTRAINT candidate_reports_candidate_id_fkey FOREIGN KEY (candidate_id) REFERENCES public.candidates(id) ON DELETE CASCADE;


--
-- Name: candidate_reports candidate_reports_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.candidate_reports
    ADD CONSTRAINT candidate_reports_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;


--
-- Name: candidates candidates_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.candidates
    ADD CONSTRAINT candidates_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;


--
-- Name: job_delete_confirmations job_delete_confirmations_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.job_delete_confirmations
    ADD CONSTRAINT job_delete_confirmations_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;


--
-- Name: job_share_links job_share_links_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.job_share_links
    ADD CONSTRAINT job_share_links_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;


--
-- Name: jobs jobs_recruiter_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.jobs
    ADD CONSTRAINT jobs_recruiter_id_fkey FOREIGN KEY (recruiter_id) REFERENCES public.recruiters(id) ON DELETE CASCADE;


--
-- Name: outreach_drafts outreach_drafts_candidate_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outreach_drafts
    ADD CONSTRAINT outreach_drafts_candidate_id_fkey FOREIGN KEY (candidate_id) REFERENCES public.candidates(id) ON DELETE CASCADE;


--
-- Name: outreach_drafts outreach_drafts_job_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outreach_drafts
    ADD CONSTRAINT outreach_drafts_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;


--
-- Name: share_link_outreach_events share_link_outreach_events_candidate_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.share_link_outreach_events
    ADD CONSTRAINT share_link_outreach_events_candidate_id_fkey FOREIGN KEY (candidate_id) REFERENCES public.candidates(id) ON DELETE CASCADE;


--
-- Name: share_link_outreach_events share_link_outreach_events_share_token_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.share_link_outreach_events
    ADD CONSTRAINT share_link_outreach_events_share_token_fkey FOREIGN KEY (share_token) REFERENCES public.job_share_links(token) ON DELETE CASCADE;


--
-- Name: usage_events usage_events_candidate_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usage_events
    ADD CONSTRAINT usage_events_candidate_id_fkey FOREIGN KEY (candidate_id) REFERENCES public.candidates(id) ON DELETE CASCADE;


--
-- Name: usage_events usage_events_recruiter_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.usage_events
    ADD CONSTRAINT usage_events_recruiter_id_fkey FOREIGN KEY (recruiter_id) REFERENCES public.recruiters(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict yv7kiqPRB7OZkKkGzZLaxhBB3gYVGVqLeuyVomjSKqRnjoAknl39imK1DUQwC3y

