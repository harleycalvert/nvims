#!/usr/bin/env python3
"""
Pull training component data from training.gov.au SOAP API (sandbox).
Outputs tga_data.json with qualifications, skill sets, units, and coverage mappings.

Usage:
    python3 tga.py [--output tga_data.json] [--delay 0.4]
"""

import json
import time
import logging
import argparse
from datetime import datetime, timezone

from zeep import Client
from zeep.wsse.username import UsernameToken
from zeep.helpers import serialize_object

logging.basicConfig(level=logging.INFO, format='%(asctime)s %(levelname)s %(message)s')
log = logging.getLogger(__name__)

WSDL = 'https://ws.sandbox.training.gov.au/Deewr.Tga.Webservices/TrainingComponentService.svc?wsdl'
USERNAME = 'WebService.Read'
PASSWORD = 'Asdf098'

# Code prefixes to sweep
CODE_PREFIXES = ['ICT', 'BSB']

# Extra title searches to catch qualifications that may live outside the ICT/BSB prefixes
TITLE_SEARCHES = ['Cyber Security']


def make_client() -> Client:
    return Client(WSDL, wsse=UsernameToken(USERNAME, PASSWORD))


def search_all_components(client: Client, types: dict,
                          page_size: int = 100, delay: float = 0.4) -> list:
    """Return all current TrainingComponentSummary dicts for given types (auto-paginated).

    The sandbox ignores the Filter field when SearchCode/SearchTitle flags are set,
    returning 0 results. Fetching everything and filtering client-side is the only
    reliable approach.
    """
    results = []
    page = 1
    while True:
        resp = client.service.Search(request={
            'Filter': '',
            'TrainingComponentTypes': types,
            'IncludeSuperseeded': False,
            'PageSize': page_size,
            'PageNumber': page,
        })
        d = serialize_object(resp)
        total = d.get('Count') or 0
        items = (d.get('Results') or {}).get('TrainingComponentSummary') or []
        if not items:
            break
        results.extend(items)
        if len(results) >= total:
            break
        page += 1
        time.sleep(delay)
    return results


def get_current_units(client: Client, code: str, delay: float = 0.4) -> list[dict]:
    """Return [{code, title}] for units in the current release of a component."""
    try:
        raw = client.service.GetDetails(request={
            'Code': code,
            'InformationRequest': {
                'ShowReleases': True,
                'ShowUnitGrid': True,
            },
        })
        d = serialize_object(raw)
        releases = (d.get('Releases') or {}).get('Release') or []
        # Prefer the Current release; fall back to most recent
        current = next((r for r in releases if r.get('Currency') == 'Current'), None)
        if current is None and releases:
            current = releases[0]
        if current is None:
            return []
        entries = (current.get('UnitGrid') or {}).get('UnitGridEntry') or []
        return [{'code': e['Code'], 'title': e.get('Title') or ''} for e in entries if e.get('Code')]
    except Exception as exc:
        log.warning('GetDetails failed for %s: %s', code, exc)
        return []


def dedupe(summaries: list) -> list:
    """Remove duplicate codes, keeping first occurrence."""
    seen = set()
    out = []
    for s in summaries:
        c = s.get('Code')
        if c and c not in seen:
            seen.add(c)
            out.append(s)
    return out


def collect_summaries(client: Client, prefixes: list[str],
                      title_keywords: list[str], types: dict,
                      delay: float) -> list:
    """Download all components of given types then filter client-side.

    Keeps items whose code starts with one of `prefixes` OR whose title
    contains one of `title_keywords` (case-insensitive).
    """
    log.info('Downloading all components for type filter ...')
    all_items = search_all_components(client, types, delay=delay)
    log.info('  Total downloaded: %d', len(all_items))

    prefixes_upper = [p.upper() for p in prefixes]
    keywords_lower = [k.lower() for k in title_keywords]

    def keep(item):
        code = (item.get('Code') or '').upper()
        title = (item.get('Title') or '').lower()
        return (
            any(code.startswith(p) for p in prefixes_upper)
            or any(k in title for k in keywords_lower)
        )

    filtered = [i for i in all_items if keep(i)]
    log.info('  After prefix/keyword filter: %d', len(filtered))
    return filtered


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', default='tga_data.json')
    parser.add_argument('--delay', type=float, default=0.4,
                        help='Seconds between API calls')
    args = parser.parse_args()

    log.info('Connecting to SOAP sandbox ...')
    client = make_client()

    qual_types         = {'IncludeQualification': True, 'IncludeSkillSet': False, 'IncludeUnit': False}
    accreted_types     = {'IncludeAccreditedCourse': True}
    ss_types           = {'IncludeQualification': False, 'IncludeSkillSet': True,  'IncludeUnit': False}
    unit_types         = {'IncludeQualification': False, 'IncludeSkillSet': False, 'IncludeUnit': True}

    # --- Discover qualifications ------------------------------------------------
    log.info('=== Discovering qualifications ===')
    qual_summaries = collect_summaries(client, CODE_PREFIXES, TITLE_SEARCHES,
                                       qual_types, args.delay)
    # Also include accredited courses matching the title keywords (e.g. 22603VIC Cert IV Cyber Security)
    log.info('=== Discovering accredited courses (title match only) ===')
    acc_summaries = collect_summaries(client, [], TITLE_SEARCHES,
                                      accreted_types, args.delay)
    qual_summaries = dedupe(qual_summaries + acc_summaries)
    log.info('Total qualifications to fetch: %d', len(qual_summaries))

    # --- Discover skill sets ----------------------------------------------------
    log.info('=== Discovering skill sets ===')
    ss_summaries = collect_summaries(client, CODE_PREFIXES, TITLE_SEARCHES,
                                     ss_types, args.delay)
    log.info('Total skill sets to fetch: %d', len(ss_summaries))

    # --- Build unit master registry (filled as we fetch details) ----------------
    unit_registry: dict[str, str] = {}  # code -> title

    # --- Fetch qualification details --------------------------------------------
    log.info('=== Fetching qualification unit lists ===')
    qualifications = []
    for i, q in enumerate(qual_summaries, 1):
        code = q['Code']
        title = q['Title']
        log.info('[%d/%d] %s %s', i, len(qual_summaries), code, title)
        units = get_current_units(client, code, args.delay)
        for u in units:
            unit_registry.setdefault(u['code'], u['title'])
        comp_types = q.get('ComponentType') or ['Qualification']
        comp_type = comp_types[0] if comp_types else 'Qualification'
        qualifications.append({
            'code': code,
            'title': title,
            'type': comp_type,
            'is_current': q.get('IsCurrent', True),
            'units': [u['code'] for u in units],
            'skill_set_coverage': [],   # filled in later
        })
        time.sleep(args.delay)

    # --- Fetch skill set details -------------------------------------------------
    log.info('=== Fetching skill set unit lists ===')
    skill_sets = []
    for i, ss in enumerate(ss_summaries, 1):
        code = ss['Code']
        title = ss['Title']
        log.info('[%d/%d] %s %s', i, len(ss_summaries), code, title)
        units = get_current_units(client, code, args.delay)
        for u in units:
            unit_registry.setdefault(u['code'], u['title'])
        skill_sets.append({
            'code': code,
            'title': title,
            'type': 'SkillSet',
            'is_current': ss.get('IsCurrent', True),
            'units': [u['code'] for u in units],
        })
        time.sleep(args.delay)

    # --- Fetch unit details for any codes not yet in the registry ---------------
    known_unit_codes = set(unit_registry.keys())
    all_referenced = set()
    for q in qualifications:
        all_referenced.update(q['units'])
    for ss in skill_sets:
        all_referenced.update(ss['units'])
    missing = all_referenced - known_unit_codes

    if missing:
        log.info('=== Fetching unit search to fill %d missing titles ===', len(missing))
        unit_summaries = search_all_components(client, unit_types, delay=args.delay)
        for u in unit_summaries:
            unit_registry.setdefault(u['Code'], u.get('Title') or '')

    # --- Compute skill-set coverage per qualification ----------------------------
    log.info('=== Computing skill set coverage ===')
    for q in qualifications:
        q_units = set(q['units'])
        covered = []
        for ss in skill_sets:
            ss_units = set(ss['units'])
            if ss_units and ss_units.issubset(q_units):
                covered.append(ss['code'])
        q['skill_set_coverage'] = covered

    # --- Assemble unit list (all referenced codes) --------------------------------
    all_unit_codes = set()
    for q in qualifications:
        all_unit_codes.update(q['units'])
    for ss in skill_sets:
        all_unit_codes.update(ss['units'])

    units_list = sorted(
        [{'code': c, 'title': unit_registry.get(c, '')} for c in all_unit_codes],
        key=lambda x: x['code'],
    )

    # --- Write output ------------------------------------------------------------
    output = {
        'generated_at': datetime.now(timezone.utc).isoformat(),
        'source': 'training.gov.au SOAP API (sandbox)',
        'counts': {
            'qualifications': len(qualifications),
            'skill_sets': len(skill_sets),
            'units': len(units_list),
        },
        'qualifications': sorted(qualifications, key=lambda x: x['code']),
        'skill_sets': sorted(skill_sets, key=lambda x: x['code']),
        'units': units_list,
        'specialisations': [],
    }

    with open(args.output, 'w') as fh:
        json.dump(output, fh, indent=2, default=str)

    log.info('Written %s', args.output)
    log.info('Qualifications: %d, Skill sets: %d, Units: %d',
             len(qualifications), len(skill_sets), len(units_list))


if __name__ == '__main__':
    main()
