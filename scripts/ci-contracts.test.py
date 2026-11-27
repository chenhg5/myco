#!/usr/bin/env python3
import importlib.util
from pathlib import Path
import unittest

spec=importlib.util.spec_from_file_location("contracts",Path(__file__).with_name("ci-contracts.py"))
contracts=importlib.util.module_from_spec(spec)
spec.loader.exec_module(contracts)


class ContractOutcomeTests(unittest.TestCase):
    def test_all_seven_original_contract_names_are_required(self):
        names=[n for group in contracts.SUITES.values() for n in group]
        self.assertEqual(set(names), {
            "TestAgentActionsSDK", "TestPageExportApplications", "TestWhiteboardSharedExport",
            "TestPaletteAgentToPage", "TestShoppingComparisonAgentToPage",
            "TestDatabaseNativeBridgeAndProductionUI", "TestRealSSHOverviewTemplateParser",
        })
        self.assertEqual(len(names),7)
        events=[dict(Test=n,Action="pass") for n in names]
        self.assertEqual(set(contracts.verify(events,names,0)),set(names))

    def test_missing_skipped_duplicate_and_failed_cases_cannot_pass(self):
        cases=[([],0),([dict(Test="TestA",Action="skip")],0),
               ([dict(Test="TestA",Action="pass")]*2,0),
               ([dict(Test="TestA",Action="pass")],1),
               ([dict(Test="TestA",Action="fail")],0)]
        for events,code in cases:
            with self.subTest(events=events,code=code),self.assertRaises(ValueError):
                contracts.verify(events,["TestA"],code)

    def test_subtest_or_package_failure_cannot_hide_behind_top_level_pass(self):
        for name in ("TestA/subcase",None):
            with self.subTest(name=name),self.assertRaises(ValueError):
                contracts.verify([dict(Test="TestA",Action="pass"),dict(Test=name,Package="example",Action="fail")],["TestA"],0)


if __name__=="__main__":unittest.main()
