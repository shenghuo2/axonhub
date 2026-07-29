package objects

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelPrice_Equals(t *testing.T) {
	d1 := decimal.NewFromFloat(0.01)
	d2 := decimal.NewFromFloat(0.02)
	upTo1000 := int64(1000)

	tests := []struct {
		name     string
		p1       ModelPrice
		p2       ModelPrice
		expected bool
	}{
		{
			name: "Equal simple",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode:         PricingModeUsagePerUnit,
							UsagePerUnit: &d1,
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode:         PricingModeUsagePerUnit,
							UsagePerUnit: &d1,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Not equal mode",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeFlatFee,
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeUsagePerUnit,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Not equal usage per unit",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode:         PricingModeUsagePerUnit,
							UsagePerUnit: &d1,
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode:         PricingModeUsagePerUnit,
							UsagePerUnit: &d2,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Equal tiered",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeTiered,
							UsageTiered: &TieredPricing{
								Tiers: []PriceTier{
									{UpTo: &upTo1000, PricePerUnit: d1},
									{UpTo: nil, PricePerUnit: d2},
								},
							},
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeTiered,
							UsageTiered: &TieredPricing{
								Tiers: []PriceTier{
									{UpTo: &upTo1000, PricePerUnit: d1},
									{UpTo: nil, PricePerUnit: d2},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Not equal tiered price",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeTiered,
							UsageTiered: &TieredPricing{
								Tiers: []PriceTier{
									{UpTo: &upTo1000, PricePerUnit: d1},
								},
							},
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeTiered,
							UsageTiered: &TieredPricing{
								Tiers: []PriceTier{
									{UpTo: &upTo1000, PricePerUnit: d2},
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Equal with variants",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeWriteCachedTokens,
						Pricing: Pricing{
							Mode: PricingModeFlatFee,
						},
						PromptWriteCacheVariants: []PromptWriteCacheVariant{
							{
								VariantCode: PromptWriteCacheVariantCode5Min,
								Pricing: Pricing{
									Mode:         PricingModeUsagePerUnit,
									UsagePerUnit: &d1,
								},
							},
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeWriteCachedTokens,
						Pricing: Pricing{
							Mode: PricingModeFlatFee,
						},
						PromptWriteCacheVariants: []PromptWriteCacheVariant{
							{
								VariantCode: PromptWriteCacheVariantCode5Min,
								Pricing: Pricing{
									Mode:         PricingModeUsagePerUnit,
									UsagePerUnit: &d1,
								},
							},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.p1.Equals(tt.p2))
			assert.Equal(t, tt.expected, tt.p2.Equals(tt.p1))
		})
	}
}

func TestModelPriceFindServiceTierMultiplierPriorityCompatibility(t *testing.T) {
	fastOnly := ModelPrice{
		ServiceTierMultipliers: []ServiceTierMultiplier{
			{ServiceTier: "fast", Multiplier: decimal.RequireFromString("2.5")},
		},
	}
	require.True(t, fastOnly.FindServiceTierMultiplier(" PRIORITY ").Equal(decimal.RequireFromString("2.5")))

	priorityOnly := ModelPrice{
		ServiceTierMultipliers: []ServiceTierMultiplier{
			{ServiceTier: "priority", Multiplier: decimal.RequireFromString("2")},
		},
	}
	require.True(t, priorityOnly.FindServiceTierMultiplier("FAST").Equal(decimal.RequireFromString("2")))

	exactWins := ModelPrice{
		ServiceTierMultipliers: []ServiceTierMultiplier{
			{ServiceTier: "fast", Multiplier: decimal.RequireFromString("2")},
			{ServiceTier: "priority", Multiplier: decimal.RequireFromString("2.5")},
		},
	}
	require.True(t, exactWins.FindServiceTierMultiplier("fast").Equal(decimal.RequireFromString("2")))
	require.True(t, exactWins.FindServiceTierMultiplier("priority").Equal(decimal.RequireFromString("2.5")))
	require.True(t, exactWins.FindServiceTierMultiplier("unknown").Equal(decimal.NewFromInt(1)))
}

func TestModelPrice_EqualsVolume(t *testing.T) {
	d1 := decimal.NewFromFloat(0.01)
	d2 := decimal.NewFromFloat(0.02)
	upTo1000 := int64(1000)

	p1 := ModelPrice{
		Items: []ModelPriceItem{
			{
				ItemCode: PriceItemCodeUsage,
				Pricing: Pricing{
					Mode: PricingModeVolume,
					UsageTiered: &TieredPricing{
						Tiers: []PriceTier{
							{UpTo: &upTo1000, PricePerUnit: d1},
							{UpTo: nil, PricePerUnit: d2},
						},
					},
				},
			},
		},
	}

	p2 := ModelPrice{
		Items: []ModelPriceItem{
			{
				ItemCode: PriceItemCodeUsage,
				Pricing: Pricing{
					Mode: PricingModeVolume,
					UsageTiered: &TieredPricing{
						Tiers: []PriceTier{
							{UpTo: &upTo1000, PricePerUnit: d1},
							{UpTo: nil, PricePerUnit: d2},
						},
					},
				},
			},
		},
	}

	assert.True(t, p1.Equals(p2))

	// Different mode should not be equal
	p3 := ModelPrice{
		Items: []ModelPriceItem{
			{
				ItemCode: PriceItemCodeUsage,
				Pricing: Pricing{
					Mode: PricingModeVolume,
					UsageTiered: &TieredPricing{
						Tiers: []PriceTier{
							{UpTo: &upTo1000, PricePerUnit: d1},
							{UpTo: nil, PricePerUnit: d2},
						},
					},
				},
			},
		},
	}

	p4 := ModelPrice{
		Items: []ModelPriceItem{
			{
				ItemCode: PriceItemCodeUsage,
				Pricing: Pricing{
					Mode: PricingModeTiered,
					UsageTiered: &TieredPricing{
						Tiers: []PriceTier{
							{UpTo: &upTo1000, PricePerUnit: d1},
							{UpTo: nil, PricePerUnit: d2},
						},
					},
				},
			},
		},
	}

	assert.False(t, p3.Equals(p4))
}

func TestModelPrice_Validate(t *testing.T) {
	t.Run("flat_fee requires flatFee", func(t *testing.T) {
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing:  Pricing{Mode: PricingModeFlatFee},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "flatFee is required")
	})

	t.Run("usage_per_unit requires usagePerUnit", func(t *testing.T) {
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing:  Pricing{Mode: PricingModeUsagePerUnit},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usagePerUnit is required")
	})

	t.Run("tiered requires last upTo null and others non-null", func(t *testing.T) {
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing: Pricing{
						Mode: PricingModeTiered,
						UsageTiered: &TieredPricing{
							Tiers: []PriceTier{
								{UpTo: nil, PricePerUnit: decimal.NewFromFloat(0.01)},
								{UpTo: nil, PricePerUnit: decimal.NewFromFloat(0.02)},
							},
						},
					},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tiers[0].upTo is required")
	})

	t.Run("tiered last upTo must be null", func(t *testing.T) {
		upTo1000 := int64(1000)
		upTo2000 := int64(2000)
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing: Pricing{
						Mode: PricingModeTiered,
						UsageTiered: &TieredPricing{
							Tiers: []PriceTier{
								{UpTo: &upTo1000, PricePerUnit: decimal.NewFromFloat(0.01)},
								{UpTo: &upTo2000, PricePerUnit: decimal.NewFromFloat(0.02)},
							},
						},
					},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tiers[1].upTo must be null")
	})

	t.Run("volume requires usageTiered", func(t *testing.T) {
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing:  Pricing{Mode: PricingModeVolume},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usageTiered is required")
	})

	t.Run("volume validates tiers same as tiered", func(t *testing.T) {
		upTo1000 := int64(1000)
		upTo2000 := int64(2000)
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing: Pricing{
						Mode: PricingModeVolume,
						UsageTiered: &TieredPricing{
							Tiers: []PriceTier{
								{UpTo: &upTo1000, PricePerUnit: decimal.NewFromFloat(0.01)},
								{UpTo: &upTo2000, PricePerUnit: decimal.NewFromFloat(0.02)},
							},
						},
					},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tiers[1].upTo must be null")
	})

	t.Run("variant pricing is also validated", func(t *testing.T) {
		d := decimal.NewFromFloat(0.01)
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeWriteCachedTokens,
					Pricing:  Pricing{Mode: PricingModeUsagePerUnit, UsagePerUnit: &d},
					PromptWriteCacheVariants: []PromptWriteCacheVariant{
						{
							VariantCode: PromptWriteCacheVariantCode5Min,
							Pricing:     Pricing{Mode: PricingModeUsagePerUnit},
						},
					},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "promptWriteCacheVariants[0]")
		assert.Contains(t, err.Error(), "usagePerUnit is required")
	})

	t.Run("service tier name is required", func(t *testing.T) {
		mp := ModelPrice{
			ServiceTierMultipliers: []ServiceTierMultiplier{
				{ServiceTier: "  ", Multiplier: decimal.NewFromInt(2)},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "serviceTier is required")
	})

	t.Run("service tier names are case insensitive", func(t *testing.T) {
		mp := ModelPrice{
			ServiceTierMultipliers: []ServiceTierMultiplier{
				{ServiceTier: "fast", Multiplier: decimal.NewFromInt(2)},
				{ServiceTier: " FAST ", Multiplier: decimal.NewFromFloat(2.5)},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "serviceTier is duplicated")
	})

	for _, multiplier := range []decimal.Decimal{decimal.Zero, decimal.NewFromInt(-1)} {
		t.Run("service tier multiplier must be positive "+multiplier.String(), func(t *testing.T) {
			mp := ModelPrice{
				ServiceTierMultipliers: []ServiceTierMultiplier{
					{ServiceTier: "fast", Multiplier: multiplier},
				},
			}

			err := mp.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "multiplier must be greater than zero")
		})
	}
}
